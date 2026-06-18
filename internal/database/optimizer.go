package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type albumClusterKey struct {
	cleanTitle  string
	cleanArtist string
}

type albumReconciliationCandidate struct {
	id        string
	hasRating bool
	rating    float64
	hasGenres bool
}

// ReconcileDuplicateAlbums merges albums sharing identical clean title/artist keys.
func ReconcileDuplicateAlbums(db *DBClient) (int64, error) {
	if db == nil || db.Ctx == nil {
		return 0, errors.New("database client is nil")
	}

	tx, err := db.Ctx.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	clusters, err := loadAlbumReconciliationClusters(tx)
	if err != nil {
		return 0, err
	}

	updateTracksStmt, err := tx.Prepare("UPDATE tracks SET album_id = ? WHERE album_id = ?")
	if err != nil {
		return 0, err
	}
	defer updateTracksStmt.Close()

	deleteAlbumStmt, err := tx.Prepare("DELETE FROM albums WHERE id = ?")
	if err != nil {
		return 0, err
	}
	defer deleteAlbumStmt.Close()

	var purgedRows int64
	for _, candidates := range clusters {
		if len(candidates) < 2 {
			continue
		}

		survivor := electAlbumSurvivor(candidates)
		for _, candidate := range candidates {
			if candidate.id == survivor.id {
				continue
			}

			if _, err := updateTracksStmt.Exec(survivor.id, candidate.id); err != nil {
				return 0, fmt.Errorf("failed to move tracks from duplicate album %q to survivor %q: %w", candidate.id, survivor.id, err)
			}

			result, err := deleteAlbumStmt.Exec(candidate.id)
			if err != nil {
				return 0, fmt.Errorf("failed to delete duplicate album %q: %w", candidate.id, err)
			}
			rowsDeleted, err := result.RowsAffected()
			if err != nil {
				return 0, fmt.Errorf("failed to count deleted duplicate album %q: %w", candidate.id, err)
			}
			if rowsDeleted != 1 {
				return 0, fmt.Errorf("deleted %d rows for duplicate album %q, want 1", rowsDeleted, candidate.id)
			}
			purgedRows += rowsDeleted
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return purgedRows, nil
}

func loadAlbumReconciliationClusters(tx *sql.Tx) (map[albumClusterKey][]albumReconciliationCandidate, error) {
	rows, err := tx.Query(`
		SELECT id, clean_title, clean_artist, genres, user_rating
		FROM albums`)
	if err != nil {
		return nil, fmt.Errorf("failed to query albums for reconciliation: %w", err)
	}
	defer rows.Close()

	clusters := make(map[albumClusterKey][]albumReconciliationCandidate)
	for rows.Next() {
		var (
			id          string
			cleanTitle  string
			cleanArtist string
			genres      sql.NullString
			userRating  sql.NullFloat64
		)

		if err := rows.Scan(&id, &cleanTitle, &cleanArtist, &genres, &userRating); err != nil {
			return nil, err
		}

		key := albumClusterKey{
			cleanTitle:  cleanTitle,
			cleanArtist: cleanArtist,
		}
		clusters[key] = append(clusters[key], albumReconciliationCandidate{
			id:        id,
			hasRating: userRating.Valid,
			rating:    userRating.Float64,
			hasGenres: hasPopulatedGenreArray(genres),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return clusters, nil
}

func electAlbumSurvivor(candidates []albumReconciliationCandidate) albumReconciliationCandidate {
	ranked := append([]albumReconciliationCandidate(nil), candidates...)
	sort.SliceStable(ranked, func(i, j int) bool {
		left := ranked[i]
		right := ranked[j]

		if left.hasRating != right.hasRating {
			return left.hasRating
		}
		if left.hasRating && right.hasRating && left.rating != right.rating {
			return left.rating > right.rating
		}
		if left.hasGenres != right.hasGenres {
			return left.hasGenres
		}
		return left.id < right.id
	})

	return ranked[0]
}

func hasPopulatedGenreArray(value sql.NullString) bool {
	if !value.Valid {
		return false
	}

	raw := strings.TrimSpace(value.String)
	if raw == "" {
		return false
	}

	var genres []any
	if err := json.Unmarshal([]byte(raw), &genres); err != nil {
		return false
	}
	return len(genres) > 0
}
