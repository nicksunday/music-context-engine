package ingest

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/nicksunday/music-context-platform/internal/database"
	"github.com/nicksunday/music-context-platform/internal/utils"
)

type LastFMIngestResult struct {
	Rows          int
	MatchedTracks int64
	CreatedTracks int
	UnmatchedRows int
}

func IngestLastFMFavorites(db *database.DBClient, filePath string, logger *log.Logger) (LastFMIngestResult, error) {
	var result LastFMIngestResult

	file, err := os.Open(filePath)
	if err != nil {
		return result, err
	}
	defer file.Close()

	reader := newCSVReader(file)

	header, err := reader.Read()
	if err != nil {
		return result, err
	}

	columns := headerColumns(header)
	if _, err := requiredColumn(columns, "uts"); err != nil {
		return result, err
	}
	if _, err := requiredColumn(columns, "track_mbid"); err != nil {
		return result, err
	}
	artistIdx, err := requiredColumn(columns, "artist")
	if err != nil {
		return result, err
	}
	trackIdx, err := requiredColumn(columns, "track")
	if err != nil {
		return result, err
	}
	albumIdx, hasAlbumColumn := optionalColumn(columns, "album")

	tx, err := db.Ctx.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	updateStmt, err := tx.Prepare(`
		UPDATE tracks
		SET is_favorite = 1,
			is_disliked = 0
		WHERE clean_title = ? AND clean_artist = ?`)
	if err != nil {
		return result, err
	}
	defer updateStmt.Close()

	insertStmt, err := tx.Prepare(`
		INSERT INTO tracks (
			id, album_id, title, album, artist, clean_title, clean_artist, is_favorite, is_disliked
		) VALUES (?, ?, ?, ?, ?, ?, ?, 1, 0)
		ON CONFLICT(id) DO UPDATE SET
			clean_title = excluded.clean_title,
			clean_artist = excluded.clean_artist,
			album_id = excluded.album_id,
			album = excluded.album,
			is_favorite = 1,
			is_disliked = 0`)
	if err != nil {
		return result, err
	}
	defer insertStmt.Close()

	createdAlbumCounts := map[string]int{}
	row := 1
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, err
		}
		row++
		result.Rows++

		title := csvField(record, trackIdx)
		artist := csvField(record, artistIdx)
		album := ""
		if hasAlbumColumn {
			album = csvField(record, albumIdx)
		}
		if title == "" || artist == "" {
			result.UnmatchedRows++
			debugf(logger, "debug: unmatched Last.fm row %d with empty title or artist", row)
			continue
		}

		cleanTitle, err := utils.NormalizeSearchText(title)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to normalize title %q: %w", row, title, err)
		}
		cleanArtist, err := utils.NormalizeSearchText(artist)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to normalize artist %q: %w", row, artist, err)
		}

		updateResult, err := updateStmt.Exec(cleanTitle, cleanArtist)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to mark Last.fm favorite %q by %q: %w", row, title, artist, err)
		}
		rowsAffected, err := updateResult.RowsAffected()
		if err != nil {
			return result, err
		}
		if rowsAffected == 0 {
			albumID, albumTitle, _, err := resolveFavoriteAlbumID(tx, album, artist)
			if err != nil {
				return result, fmt.Errorf("row %d: failed to resolve Last.fm album for %q by %q: %w", row, title, artist, err)
			}

			if _, err := insertStmt.Exec(trackID(title, artist, albumTitle), albumID, title, albumTitle, artist, cleanTitle, cleanArtist); err != nil {
				return result, fmt.Errorf("row %d: failed to create Last.fm track %q by %q: %w", row, title, artist, err)
			}
			createdAlbumCounts[albumID] = 1
			result.CreatedTracks++
			continue
		}

		result.MatchedTracks += rowsAffected
	}

	if err := refreshAlbumTrackCounts(tx, createdAlbumCounts); err != nil {
		return result, err
	}

	if err := tx.Commit(); err != nil {
		return result, err
	}

	return result, nil
}

func resolveFavoriteAlbumID(tx *sql.Tx, albumTitle, artist string) (string, string, bool, error) {
	if albumTitle == "" {
		albumTitle = "Unknown Album"
	}

	cleanTitle, err := utils.NormalizeSearchText(albumTitle)
	if err != nil {
		return "", "", false, fmt.Errorf("failed to normalize album title %q: %w", albumTitle, err)
	}
	cleanArtist, err := utils.NormalizeSearchText(artist)
	if err != nil {
		return "", "", false, fmt.Errorf("failed to normalize album artist %q: %w", artist, err)
	}

	var id string
	err = tx.QueryRow(
		"SELECT id FROM albums WHERE clean_title = ? AND clean_artist = ? LIMIT 1",
		cleanTitle,
		cleanArtist,
	).Scan(&id)
	if err == nil {
		return id, albumTitle, false, nil
	}
	if err != sql.ErrNoRows {
		return "", "", false, err
	}

	id = albumID(cleanTitle, cleanArtist)
	_, err = tx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			clean_title = excluded.clean_title,
			clean_artist = excluded.clean_artist`,
		id,
		albumTitle,
		artist,
		cleanTitle,
		cleanArtist,
	)
	if err != nil {
		return "", "", false, err
	}

	return id, albumTitle, true, nil
}

func debugf(logger *log.Logger, format string, args ...any) {
	if logger == nil {
		return
	}
	logger.Printf(format, args...)
}
