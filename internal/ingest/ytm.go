package ingest

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nicksunday/music-context-platform/internal/database"
	"github.com/nicksunday/music-context-platform/internal/utils"
)

func IngestYTMLibrary(db *database.DBClient, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := newCSVReader(file)

	// Skip header row
	if _, err := reader.Read(); err != nil {
		return err
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		title := strings.TrimSpace(record[1])
		album := strings.TrimSpace(record[2])
		artist := strings.TrimSpace(record[3])

		if title == "" || artist == "" {
			continue
		}

		cleanTitle, err := utils.NormalizeSearchText(title)
		if err != nil {
			return fmt.Errorf("failed to normalize title %q: %w", title, err)
		}
		cleanArtist, err := utils.NormalizeSearchText(artist)
		if err != nil {
			return fmt.Errorf("failed to normalize artist %q: %w", artist, err)
		}

		query := `
				INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist)
				VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				clean_title = excluded.clean_title,
				clean_artist = excluded.clean_artist;`

		_, err = db.Ctx.Exec(query, trackID(title, artist, album), title, album, artist, cleanTitle, cleanArtist)
		if err != nil {
			return fmt.Errorf("failed to insert %s: %w", title, err)
		}
	}

	return nil
}

func IngestYT_Uploads(db *database.DBClient, filePath string) error {
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
		columns[ytmHeaderKey(name)] = i
	}

	titleIdx, err := requiredYTMColumn(columns, "Song Title")
	if err != nil {
		return err
	}
	albumIdx, err := requiredYTMColumn(columns, "Album Title")
	if err != nil {
		return err
	}
	artistIdx, err := requiredYTMColumn(columns, "Artist Name 1")
	if err != nil {
		return err
	}

	query := `
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			clean_title = excluded.clean_title,
			clean_artist = excluded.clean_artist;`

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

		title := strings.TrimSpace(ytmField(record, titleIdx))
		album := strings.TrimSpace(ytmField(record, albumIdx))
		artist := strings.TrimSpace(ytmField(record, artistIdx))
		if artist == "" {
			artist = "Unknown Artist"
		}
		if title == "" {
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

		if _, err := db.Ctx.Exec(query, trackID(title, artist, album), title, album, artist, cleanTitle, cleanArtist); err != nil {
			return fmt.Errorf("row %d: failed to insert upload %q: %w", row, title, err)
		}
	}

	return nil
}

func ytmHeaderKey(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "\ufeff")))
}

func requiredYTMColumn(columns map[string]int, name string) (int, error) {
	idx, ok := columns[ytmHeaderKey(name)]
	if !ok {
		return 0, fmt.Errorf("missing required YouTube Music column %q", name)
	}
	return idx, nil
}

func ytmField(record []string, idx int) string {
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}
