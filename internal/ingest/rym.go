package ingest

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/nicksunday/music-context-platform/internal/database"
	"github.com/nicksunday/music-context-platform/internal/utils"
)

func IngestRYMExport(db *database.DBClient, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := newCSVReader(file)

	header, err := reader.Read()
	if err != nil {
		return err
	}

	columns := make(map[string]int, len(header))
	for i, name := range header {
		columns[rymHeaderKey(name)] = i
	}

	albumIDIdx, ok := optionalColumn(columns, "RYM Album ID", "RYM Album")
	if !ok {
		return fmt.Errorf("missing required RYM column %q", "RYM Album ID")
	}
	titleIdx, err := requiredRYMColumn(columns, "Title")
	if err != nil {
		return err
	}
	firstNameIdx, err := requiredRYMColumn(columns, "First Name")
	if err != nil {
		return err
	}
	lastNameIdx, err := requiredRYMColumn(columns, "Last Name")
	if err != nil {
		return err
	}
	releaseDateIdx, err := requiredRYMColumn(columns, "Release_Date")
	if err != nil {
		return err
	}
	ratingIdx, err := requiredRYMColumn(columns, "Rating")
	if err != nil {
		return err
	}

	query := `
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, user_rating, release_date)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			clean_title = excluded.clean_title,
			clean_artist = excluded.clean_artist,
			user_rating = excluded.user_rating,
			release_date = excluded.release_date;`

	row := 1
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		row++

		title := strings.TrimSpace(rymField(record, titleIdx))
		artist := combineArtistName(rymField(record, firstNameIdx), rymField(record, lastNameIdx))
		if title == "" || artist == "" {
			continue
		}

		cleanTitle, err := utils.NormalizeSearchText(title)
		if err != nil {
			return fmt.Errorf("row %d: failed to normalize title %q: %w", row, title, err)
		}
		cleanArtist, err := utils.NormalizeSearchText(artist)
		if err != nil {
			return fmt.Errorf("row %d: failed to normalize artist %q: %w", row, artist, err)
		}

		rating, err := parseRYMRating(rymField(record, ratingIdx))
		if err != nil {
			return fmt.Errorf("row %d: invalid Rating: %w", row, err)
		}

		releaseYear, err := parseRYMReleaseYear(rymField(record, releaseDateIdx))
		if err != nil {
			return fmt.Errorf("row %d: invalid Release_Date: %w", row, err)
		}

		albumID := strings.TrimSpace(rymField(record, albumIDIdx))
		if albumID == "" {
			albumID = rymAlbumID(title, artist)
		}
		if _, err := db.Ctx.Exec(query, albumID, title, artist, cleanTitle, cleanArtist, rating, releaseYear); err != nil {
			return fmt.Errorf("row %d: failed to upsert album %q by %q: %w", row, title, artist, err)
		}
	}

	return nil
}

func rymHeaderKey(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "\ufeff")))
}

func requiredRYMColumn(columns map[string]int, name string) (int, error) {
	idx, ok := columns[rymHeaderKey(name)]
	if !ok {
		return 0, fmt.Errorf("missing required RYM column %q", name)
	}
	return idx, nil
}

func rymField(record []string, idx int) string {
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

func combineArtistName(firstName, lastName string) string {
	parts := make([]string, 0, 2)
	if firstName = strings.TrimSpace(firstName); firstName != "" {
		parts = append(parts, firstName)
	}
	if lastName = strings.TrimSpace(lastName); lastName != "" {
		parts = append(parts, lastName)
	}
	return strings.Join(parts, " ")
}

func parseRYMRating(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	rawRating, err := strconv.Atoi(value)
	if err != nil {
		return nil, err
	}

	return float64(rawRating) / 2.0, nil
}

func parseRYMReleaseYear(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	if len(value) > 4 {
		if _, err := strconv.Atoi(value[:4]); err == nil {
			value = value[:4]
		}
	}

	releaseYear, err := strconv.Atoi(value)
	if err != nil {
		return nil, err
	}

	return releaseYear, nil
}

func rymAlbumID(title, artist string) string {
	key := strings.ToLower(strings.TrimSpace(title)) + "-" + strings.ToLower(strings.TrimSpace(artist))
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", sum)
}
