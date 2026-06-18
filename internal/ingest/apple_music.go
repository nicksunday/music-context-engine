package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nicksunday/music-context-platform/internal/database"
	"github.com/nicksunday/music-context-platform/internal/utils"
)

const (
	appleMusicNameIndex   = 0
	appleMusicArtistIndex = 1
	appleMusicAlbumIndex  = 3
)

type AppleMusicIngestResult struct {
	Rows          int
	MatchedTracks int64
	CreatedTracks int
	UnmatchedRows int
}

type AppleMusicFavoritesIngestResult struct {
	Rows          int
	MatchedTracks int64
	CreatedTracks int
	LikedRows     int
	DislikedRows  int
	UnmatchedRows int
}

type AppleMusicLibraryActivityIngestResult struct {
	Rows               int
	TrackRows          int
	MatchedTracks      int64
	CreatedTracks      int
	PositiveRows       int
	DislikedRows       int
	UnmatchedTrackRows int
	AlbumsUpdated      int
}

type AppleMusicLibraryTracksIngestResult struct {
	Rows          int
	MatchedTracks int64
	CreatedTracks int
	PositiveRows  int
	DislikedRows  int
	UnmatchedRows int
	AlbumsUpdated int
}

type AppleMusicPlayActivityIngestResult struct {
	Rows               int
	WrittenRows        int64
	SkippedAmbientRows int
	UnmatchedRows      int
}

type AppleMusicTrackPlayHistoryIngestResult struct {
	Rows               int
	WrittenRows        int64
	SkippedAmbientRows int
	UnmatchedRows      int
}

func IngestAppleMusicLikes(db *database.DBClient, filePath string) (AppleMusicIngestResult, error) {
	var result AppleMusicIngestResult

	file, err := os.Open(filePath)
	if err != nil {
		return result, err
	}
	defer file.Close()

	reader := newCSVReader(file)
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		return result, err
	}
	if !hasHeaderPrefix(header, "Name", "Artist", "Composer", "Album") {
		return result, fmt.Errorf("missing Apple Music header signature %q", "Name,Artist,Composer,Album")
	}

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

		title := csvField(record, appleMusicNameIndex)
		artist := csvField(record, appleMusicArtistIndex)
		album := csvField(record, appleMusicAlbumIndex)
		if title == "" {
			result.UnmatchedRows++
			continue
		}
		if artist == "" {
			artist = "Unknown Artist"
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
			return result, fmt.Errorf("row %d: failed to mark Apple Music favorite %q by %q: %w", row, title, artist, err)
		}
		rowsAffected, err := updateResult.RowsAffected()
		if err != nil {
			return result, err
		}
		if rowsAffected > 0 {
			result.MatchedTracks += rowsAffected
			continue
		}

		albumID, albumTitle, _, err := resolveFavoriteAlbumID(tx, album, artist)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to resolve Apple Music album for %q by %q: %w", row, title, artist, err)
		}
		if _, err := insertStmt.Exec(trackID(title, artist, albumTitle), albumID, title, albumTitle, artist, cleanTitle, cleanArtist); err != nil {
			return result, fmt.Errorf("row %d: failed to create Apple Music track %q by %q: %w", row, title, artist, err)
		}
		createdAlbumCounts[albumID] = 1
		result.CreatedTracks++
	}

	if err := refreshAlbumTrackCounts(tx, createdAlbumCounts); err != nil {
		return result, err
	}

	if err := tx.Commit(); err != nil {
		return result, err
	}

	return result, nil
}

func IngestAppleMusicFavorites(db *database.DBClient, filePath string) (AppleMusicFavoritesIngestResult, error) {
	var result AppleMusicFavoritesIngestResult

	file, err := os.Open(filePath)
	if err != nil {
		return result, err
	}
	defer file.Close()

	reader := newCSVReader(file)
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		return result, err
	}
	columns := headerColumns(header)
	favoriteTypeIdx, err := requiredColumn(columns, "Favorite Type")
	if err != nil {
		return result, err
	}
	itemDescriptionIdx, err := requiredColumn(columns, "Item Description")
	if err != nil {
		return result, err
	}
	preferenceIdx, err := requiredColumn(columns, "Preference")
	if err != nil {
		return result, err
	}

	tx, err := db.Ctx.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	updateStmt, err := tx.Prepare(`
		UPDATE tracks
		SET is_favorite = ?,
			is_disliked = ?
		WHERE clean_title = ? AND clean_artist = ?`)
	if err != nil {
		return result, err
	}
	defer updateStmt.Close()

	insertStmt, err := tx.Prepare(`
		INSERT INTO tracks (
			id, album_id, title, album, artist, clean_title, clean_artist, is_favorite, is_disliked
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			clean_title = excluded.clean_title,
			clean_artist = excluded.clean_artist,
			album_id = excluded.album_id,
			album = excluded.album,
			is_favorite = excluded.is_favorite,
			is_disliked = excluded.is_disliked`)
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

		favoriteType := strings.ToLower(csvField(record, favoriteTypeIdx))
		if favoriteType != "" && favoriteType != "song" {
			result.UnmatchedRows++
			continue
		}

		artist, title, ok := parseAppleMusicArtistTitleDescription(csvField(record, itemDescriptionIdx))
		if !ok {
			result.UnmatchedRows++
			continue
		}

		isFavorite, isDisliked, ok := appleMusicPreferenceFlags(csvField(record, preferenceIdx))
		if !ok {
			result.UnmatchedRows++
			continue
		}
		if isDisliked == 1 {
			result.DislikedRows++
		} else if isFavorite == 1 {
			result.LikedRows++
		}

		cleanTitle, err := utils.NormalizeSearchText(title)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to normalize title %q: %w", row, title, err)
		}
		cleanArtist, err := utils.NormalizeSearchText(artist)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to normalize artist %q: %w", row, artist, err)
		}

		updateResult, err := updateStmt.Exec(isFavorite, isDisliked, cleanTitle, cleanArtist)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to mark Apple Music favorite %q by %q: %w", row, title, artist, err)
		}
		rowsAffected, err := updateResult.RowsAffected()
		if err != nil {
			return result, err
		}
		if rowsAffected > 0 {
			result.MatchedTracks += rowsAffected
			continue
		}

		albumID, albumTitle, _, err := resolveFavoriteAlbumID(tx, "", artist)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to resolve Apple Music favorite album for %q by %q: %w", row, title, artist, err)
		}
		if _, err := insertStmt.Exec(trackID(title, artist, albumTitle), albumID, title, albumTitle, artist, cleanTitle, cleanArtist, isFavorite, isDisliked); err != nil {
			return result, fmt.Errorf("row %d: failed to create Apple Music favorite track %q by %q: %w", row, title, artist, err)
		}
		createdAlbumCounts[albumID] = 1
		result.CreatedTracks++
	}

	if err := refreshAlbumTrackCounts(tx, createdAlbumCounts); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}

	return result, nil
}

func IngestAppleMusicLibraryActivity(db *database.DBClient, filePath string) (AppleMusicLibraryActivityIngestResult, error) {
	var result AppleMusicLibraryActivityIngestResult

	file, err := os.Open(filePath)
	if err != nil {
		return result, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return result, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '[' {
		return result, fmt.Errorf("Apple Music library activity JSON must be a top-level array")
	}

	tx, err := db.Ctx.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	upsertAlbumStmt, err := tx.Prepare(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			clean_title = excluded.clean_title,
			clean_artist = excluded.clean_artist`)
	if err != nil {
		return result, err
	}
	defer upsertAlbumStmt.Close()

	updateTrackStmt, err := tx.Prepare(`
		UPDATE tracks
		SET album_id = ?,
			album = ?,
			title = ?,
			artist = ?,
			clean_title = ?,
			clean_artist = ?,
			is_favorite = CASE WHEN ? = 1 THEN ? ELSE COALESCE(is_favorite, 0) END,
			is_disliked = CASE WHEN ? = 1 THEN ? ELSE COALESCE(is_disliked, 0) END
		WHERE clean_title = ? AND clean_artist = ?`)
	if err != nil {
		return result, err
	}
	defer updateTrackStmt.Close()

	insertTrackStmt, err := tx.Prepare(`
		INSERT INTO tracks (
			id, album_id, title, album, artist, clean_title, clean_artist, is_favorite, is_disliked
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			album_id = excluded.album_id,
			album = excluded.album,
			title = excluded.title,
			artist = excluded.artist,
			clean_title = excluded.clean_title,
			clean_artist = excluded.clean_artist,
			is_favorite = CASE WHEN ? = 1 THEN excluded.is_favorite ELSE COALESCE(is_favorite, 0) END,
			is_disliked = CASE WHEN ? = 1 THEN excluded.is_disliked ELSE COALESCE(is_disliked, 0) END`)
	if err != nil {
		return result, err
	}
	defer insertTrackStmt.Close()

	affectedAlbumCounts := map[string]int{}
	albumTrackUnits := map[string]map[string]bool{}
	trackIdentities := map[string]appleMusicTrackIdentity{}
	for decoder.More() {
		var transaction map[string]any
		if err := decoder.Decode(&transaction); err != nil {
			return result, err
		}
		result.Rows++

		for _, record := range appleMusicTransactionTracks(transaction) {
			result.TrackRows++

			identity, hasIdentity, err := appleMusicTrackIdentityFromRecord(record, trackIdentities)
			if err != nil {
				return result, fmt.Errorf("transaction %d track %d: %w", result.Rows, result.TrackRows, err)
			}
			if !hasIdentity {
				result.UnmatchedTrackRows++
				continue
			}

			if identity.albumID == "" {
				albumID, err := upsertAppleMusicAlbum(upsertAlbumStmt, identity.album, identity.artist)
				if err != nil {
					return result, fmt.Errorf("transaction %d track %d: failed to upsert album %q by %q: %w", result.Rows, result.TrackRows, identity.album, identity.artist, err)
				}
				identity.albumID = albumID
			}

			if identity.identifier != "" {
				trackIdentities[identity.identifier] = identity
			}
			if albumTrackUnits[identity.albumID] == nil {
				albumTrackUnits[identity.albumID] = map[string]bool{}
			}
			trackUnitKey := firstNonEmpty(identity.identifier, trackID(identity.title, identity.artist, identity.album))
			albumTrackUnits[identity.albumID][trackUnitKey] = true
			parsedTrackCount := len(albumTrackUnits[identity.albumID])
			if declaredTrackCount, ok := jsonIntField(record, "Track Count On Album"); ok {
				parsedTrackCount = maxInt(parsedTrackCount, declaredTrackCount)
			}
			affectedAlbumCounts[identity.albumID] = maxInt(affectedAlbumCounts[identity.albumID], parsedTrackCount)

			isFavorite, isDisliked, hasAffinity := appleMusicAffinityFlags(record)
			switch {
			case isDisliked == 1:
				result.DislikedRows++
			case isFavorite == 1:
				result.PositiveRows++
			}
			hasAffinityFlag := boolFlag(hasAffinity)

			updateResult, err := updateTrackStmt.Exec(
				identity.albumID,
				identity.album,
				identity.title,
				identity.artist,
				identity.cleanTitle,
				identity.cleanArtist,
				hasAffinityFlag,
				isFavorite,
				hasAffinityFlag,
				isDisliked,
				identity.cleanTitle,
				identity.cleanArtist,
			)
			if err != nil {
				return result, fmt.Errorf("transaction %d track %d: failed to update Apple Music library activity track %q by %q: %w", result.Rows, result.TrackRows, identity.title, identity.artist, err)
			}
			rowsAffected, err := updateResult.RowsAffected()
			if err != nil {
				return result, err
			}
			if rowsAffected > 0 {
				result.MatchedTracks += rowsAffected
				continue
			}

			if _, err := insertTrackStmt.Exec(
				trackID(identity.title, identity.artist, identity.album),
				identity.albumID,
				identity.title,
				identity.album,
				identity.artist,
				identity.cleanTitle,
				identity.cleanArtist,
				isFavorite,
				isDisliked,
				hasAffinityFlag,
				hasAffinityFlag,
			); err != nil {
				return result, fmt.Errorf("transaction %d track %d: failed to insert Apple Music library activity track %q by %q: %w", result.Rows, result.TrackRows, identity.title, identity.artist, err)
			}
			result.CreatedTracks++
		}
	}
	if _, err := decoder.Token(); err != nil {
		return result, err
	}

	if err := refreshAlbumTrackCounts(tx, affectedAlbumCounts); err != nil {
		return result, err
	}
	result.AlbumsUpdated = len(affectedAlbumCounts)

	if err := tx.Commit(); err != nil {
		return result, err
	}

	return result, nil
}

func IngestAppleMusicLibraryTracks(db *database.DBClient, filePath string) (AppleMusicLibraryTracksIngestResult, error) {
	var result AppleMusicLibraryTracksIngestResult

	file, err := os.Open(filePath)
	if err != nil {
		return result, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return result, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '[' {
		return result, fmt.Errorf("Apple Music library tracks JSON must be a top-level array")
	}

	tx, err := db.Ctx.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	upsertAlbumStmt, err := tx.Prepare(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			clean_title = excluded.clean_title,
			clean_artist = excluded.clean_artist`)
	if err != nil {
		return result, err
	}
	defer upsertAlbumStmt.Close()

	updateTrackStmt, err := tx.Prepare(`
		UPDATE tracks
		SET album_id = ?,
			album = ?,
			title = ?,
			artist = ?,
			clean_title = ?,
			clean_artist = ?,
			is_favorite = CASE WHEN ? = 1 THEN ? ELSE COALESCE(is_favorite, 0) END,
			is_disliked = CASE WHEN ? = 1 THEN ? ELSE COALESCE(is_disliked, 0) END
		WHERE clean_title = ? AND clean_artist = ?`)
	if err != nil {
		return result, err
	}
	defer updateTrackStmt.Close()

	insertTrackStmt, err := tx.Prepare(`
		INSERT INTO tracks (
			id, album_id, title, album, artist, clean_title, clean_artist, is_favorite, is_disliked
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			album_id = excluded.album_id,
			album = excluded.album,
			title = excluded.title,
			artist = excluded.artist,
			clean_title = excluded.clean_title,
			clean_artist = excluded.clean_artist,
			is_favorite = CASE WHEN ? = 1 THEN excluded.is_favorite ELSE COALESCE(is_favorite, 0) END,
			is_disliked = CASE WHEN ? = 1 THEN excluded.is_disliked ELSE COALESCE(is_disliked, 0) END`)
	if err != nil {
		return result, err
	}
	defer insertTrackStmt.Close()

	affectedAlbumCounts := map[string]int{}
	albumTrackUnits := map[string]map[string]bool{}
	for decoder.More() {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			return result, err
		}
		result.Rows++

		title := jsonStringField(record, "Title")
		artist := firstNonEmpty(jsonStringField(record, "Artist"), jsonStringField(record, "Album Artist"), "Unknown Artist")
		album := jsonStringField(record, "Album")
		if title == "" {
			result.UnmatchedRows++
			continue
		}
		if album == "" {
			album = "Unknown Album"
		}

		cleanTitle, err := utils.NormalizeSearchText(title)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to normalize title %q: %w", result.Rows, title, err)
		}
		cleanArtist, err := utils.NormalizeSearchText(artist)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to normalize artist %q: %w", result.Rows, artist, err)
		}

		albumID, err := upsertAppleMusicAlbum(upsertAlbumStmt, album, artist)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to upsert album %q by %q: %w", result.Rows, album, artist, err)
		}
		if albumTrackUnits[albumID] == nil {
			albumTrackUnits[albumID] = map[string]bool{}
		}
		trackUnitKey := firstNonEmpty(jsonStringField(record, "Track Identifier"), trackID(title, artist, album))
		albumTrackUnits[albumID][trackUnitKey] = true
		parsedTrackCount := len(albumTrackUnits[albumID])
		if declaredTrackCount, ok := jsonIntField(record, "Track Count On Album"); ok {
			parsedTrackCount = maxInt(parsedTrackCount, declaredTrackCount)
		}
		affectedAlbumCounts[albumID] = maxInt(affectedAlbumCounts[albumID], parsedTrackCount)

		isFavorite, isDisliked, hasAffinity := appleMusicAffinityFlags(record)
		switch {
		case isDisliked == 1:
			result.DislikedRows++
		case isFavorite == 1:
			result.PositiveRows++
		}
		hasAffinityFlag := boolFlag(hasAffinity)

		updateResult, err := updateTrackStmt.Exec(
			albumID,
			album,
			title,
			artist,
			cleanTitle,
			cleanArtist,
			hasAffinityFlag,
			isFavorite,
			hasAffinityFlag,
			isDisliked,
			cleanTitle,
			cleanArtist,
		)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to update Apple Music library track %q by %q: %w", result.Rows, title, artist, err)
		}
		rowsAffected, err := updateResult.RowsAffected()
		if err != nil {
			return result, err
		}
		if rowsAffected > 0 {
			result.MatchedTracks += rowsAffected
			continue
		}

		if _, err := insertTrackStmt.Exec(
			trackID(title, artist, album),
			albumID,
			title,
			album,
			artist,
			cleanTitle,
			cleanArtist,
			isFavorite,
			isDisliked,
			hasAffinityFlag,
			hasAffinityFlag,
		); err != nil {
			return result, fmt.Errorf("row %d: failed to insert Apple Music library track %q by %q: %w", result.Rows, title, artist, err)
		}
		result.CreatedTracks++
	}
	if _, err := decoder.Token(); err != nil {
		return result, err
	}

	if err := refreshAlbumTrackCounts(tx, affectedAlbumCounts); err != nil {
		return result, err
	}
	result.AlbumsUpdated = len(affectedAlbumCounts)

	if err := tx.Commit(); err != nil {
		return result, err
	}

	return result, nil
}

func IngestAppleMusicPlayActivity(db *database.DBClient, filePath string) (AppleMusicPlayActivityIngestResult, error) {
	var result AppleMusicPlayActivityIngestResult

	file, err := os.Open(filePath)
	if err != nil {
		return result, err
	}
	defer file.Close()

	reader := newCSVReader(file)
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		return result, err
	}
	columns := headerColumns(header)

	eventTimestampIdx, err := requiredColumn(columns, "Event Timestamp")
	if err != nil {
		return result, err
	}
	playDurationIdx, err := requiredColumn(columns, "Play Duration Milliseconds")
	if err != nil {
		return result, err
	}
	endReasonIdx, err := requiredColumn(columns, "End Reason Type")
	if err != nil {
		return result, err
	}
	trackDescriptionIdx, hasTrackDescription := optionalColumn(columns, "Track Description", "Song Name")
	if !hasTrackDescription {
		return result, fmt.Errorf("missing required Apple Music play activity column %q or %q", "Track Description", "Song Name")
	}
	songNameIdx, hasSongName := optionalColumn(columns, "Song Name")
	artistIdx, hasArtist := optionalColumn(columns, "Artist", "Artist Name", "Container Artist Name")
	albumIdx, hasAlbum := optionalColumn(columns, "Album Name", "Container Album Name")
	containerNameIdx, hasContainerName := optionalColumn(columns, "Container Name")
	eventIDIdx, hasEventID := optionalColumn(columns, "Event ID")

	tx, err := db.Ctx.BeginTx(context.Background(), nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	insertStmt, err := tx.Prepare(`
		INSERT INTO apple_music_play_activity (
			id,
			event_timestamp,
			track_description,
			song_name,
			artist,
			album,
			container_name,
			play_duration_ms,
			end_reason_type,
			was_skipped
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			event_timestamp = excluded.event_timestamp,
			track_description = excluded.track_description,
			song_name = excluded.song_name,
			artist = excluded.artist,
			album = excluded.album,
			container_name = excluded.container_name,
			play_duration_ms = excluded.play_duration_ms,
			end_reason_type = excluded.end_reason_type,
			was_skipped = excluded.was_skipped`)
	if err != nil {
		return result, err
	}
	defer insertStmt.Close()

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

		eventTimestamp := csvField(record, eventTimestampIdx)
		trackDescription := csvField(record, trackDescriptionIdx)
		songName := ""
		if hasSongName {
			songName = csvField(record, songNameIdx)
		}
		if songName == "" {
			songName = trackDescription
		}

		artist := ""
		if hasArtist {
			artist = csvField(record, artistIdx)
		}
		album := ""
		if hasAlbum {
			album = csvField(record, albumIdx)
		}
		containerName := ""
		if hasContainerName {
			containerName = csvField(record, containerNameIdx)
		}

		if isAmbientNoiseTelemetry(containerName, trackDescription, artist) {
			result.SkippedAmbientRows++
			continue
		}
		if eventTimestamp == "" || trackDescription == "" {
			result.UnmatchedRows++
			continue
		}

		playDuration, err := parseOptionalInt64(csvField(record, playDurationIdx))
		if err != nil {
			return result, fmt.Errorf("row %d: invalid Play Duration Milliseconds: %w", row, err)
		}
		endReason := csvField(record, endReasonIdx)
		wasSkipped := 0
		if strings.Contains(strings.ToUpper(endReason), "SKIP") {
			wasSkipped = 1
		}

		id := ""
		if hasEventID {
			id = csvField(record, eventIDIdx)
		}
		if id == "" {
			id = appleMusicPlayActivityID(eventTimestamp, trackDescription, csvField(record, playDurationIdx), endReason)
		}

		insertResult, err := insertStmt.Exec(
			id,
			eventTimestamp,
			trackDescription,
			songName,
			nullableText(artist),
			nullableText(album),
			nullableText(containerName),
			playDuration,
			endReason,
			wasSkipped,
		)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to insert Apple Music play activity row: %w", row, err)
		}
		rowsAffected, err := insertResult.RowsAffected()
		if err != nil {
			return result, err
		}
		result.WrittenRows += rowsAffected
	}

	if err := tx.Commit(); err != nil {
		return result, err
	}

	return result, nil
}

func IngestAppleMusicTrackPlayHistory(db *database.DBClient, filePath string) (AppleMusicTrackPlayHistoryIngestResult, error) {
	var result AppleMusicTrackPlayHistoryIngestResult

	file, err := os.Open(filePath)
	if err != nil {
		return result, err
	}
	defer file.Close()

	reader := newCSVReader(file)
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		return result, err
	}
	columns := headerColumns(header)

	trackNameIdx, err := requiredColumn(columns, "Track Name")
	if err != nil {
		return result, err
	}
	lastPlayedIdx, err := requiredColumn(columns, "Last Played Date")
	if err != nil {
		return result, err
	}
	userInitiatedIdx, err := requiredColumn(columns, "Is User Initiated")
	if err != nil {
		return result, err
	}

	tx, err := db.Ctx.BeginTx(context.Background(), nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	insertStmt, err := tx.Prepare(`
		INSERT INTO apple_music_play_activity (
			id,
			event_timestamp,
			track_description,
			song_name,
			artist,
			end_reason_type,
			was_skipped,
			source_type,
			is_user_initiated
		) VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			event_timestamp = excluded.event_timestamp,
			track_description = excluded.track_description,
			song_name = excluded.song_name,
			artist = excluded.artist,
			end_reason_type = excluded.end_reason_type,
			was_skipped = excluded.was_skipped,
			source_type = excluded.source_type,
			is_user_initiated = excluded.is_user_initiated`)
	if err != nil {
		return result, err
	}
	defer insertStmt.Close()

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

		trackName := csvField(record, trackNameIdx)
		artist, title, ok := parseAppleMusicArtistTitleDescription(trackName)
		if !ok {
			result.UnmatchedRows++
			continue
		}
		if isAmbientNoiseTelemetry(trackName, artist) {
			result.SkippedAmbientRows++
			continue
		}

		eventTimestamp, err := parseAppleMusicEpochMillis(csvField(record, lastPlayedIdx))
		if err != nil {
			return result, fmt.Errorf("row %d: invalid Last Played Date: %w", row, err)
		}
		if eventTimestamp == "" {
			result.UnmatchedRows++
			continue
		}

		userInitiated, err := parseOptionalBoolFlag(csvField(record, userInitiatedIdx))
		if err != nil {
			return result, fmt.Errorf("row %d: invalid Is User Initiated: %w", row, err)
		}
		id := appleMusicPlayActivityID(eventTimestamp, trackName, csvField(record, userInitiatedIdx), "TRACK_PLAY_HISTORY")

		insertResult, err := insertStmt.Exec(
			id,
			eventTimestamp,
			trackName,
			title,
			artist,
			"TRACK_PLAY_HISTORY",
			"track_play_history",
			userInitiated,
		)
		if err != nil {
			return result, fmt.Errorf("row %d: failed to insert Apple Music track play history row: %w", row, err)
		}
		rowsAffected, err := insertResult.RowsAffected()
		if err != nil {
			return result, err
		}
		result.WrittenRows += rowsAffected
	}

	if err := tx.Commit(); err != nil {
		return result, err
	}

	return result, nil
}

func upsertAppleMusicAlbum(stmt *sql.Stmt, albumTitle, artist string) (string, error) {
	cleanTitle, err := utils.NormalizeSearchText(albumTitle)
	if err != nil {
		return "", fmt.Errorf("failed to normalize album title %q: %w", albumTitle, err)
	}
	cleanArtist, err := utils.NormalizeSearchText(artist)
	if err != nil {
		return "", fmt.Errorf("failed to normalize album artist %q: %w", artist, err)
	}

	id := albumID(cleanTitle, cleanArtist)
	if _, err := stmt.Exec(id, albumTitle, artist, cleanTitle, cleanArtist); err != nil {
		return "", err
	}

	return id, nil
}

type appleMusicTrackIdentity struct {
	identifier  string
	albumID     string
	title       string
	album       string
	artist      string
	cleanTitle  string
	cleanArtist string
}

func appleMusicTransactionTracks(transaction map[string]any) []map[string]any {
	rawTracks, ok := transaction["Tracks"].([]any)
	if !ok {
		return nil
	}

	tracks := make([]map[string]any, 0, len(rawTracks))
	for _, rawTrack := range rawTracks {
		track, ok := rawTrack.(map[string]any)
		if ok {
			tracks = append(tracks, track)
		}
	}
	return tracks
}

func appleMusicTrackIdentityFromRecord(record map[string]any, known map[string]appleMusicTrackIdentity) (appleMusicTrackIdentity, bool, error) {
	identifier := jsonStringField(record, "Track Identifier")
	if identifier != "" {
		if identity, ok := known[identifier]; ok && jsonStringField(record, "Title") == "" {
			return identity, true, nil
		}
	}

	title := jsonStringField(record, "Title")
	if title == "" {
		return appleMusicTrackIdentity{}, false, nil
	}
	artist := firstNonEmpty(jsonStringField(record, "Artist"), jsonStringField(record, "Album Artist"), "Unknown Artist")
	album := firstNonEmpty(jsonStringField(record, "Album"), "Unknown Album")

	cleanTitle, err := utils.NormalizeSearchText(title)
	if err != nil {
		return appleMusicTrackIdentity{}, false, fmt.Errorf("failed to normalize title %q: %w", title, err)
	}
	cleanArtist, err := utils.NormalizeSearchText(artist)
	if err != nil {
		return appleMusicTrackIdentity{}, false, fmt.Errorf("failed to normalize artist %q: %w", artist, err)
	}

	return appleMusicTrackIdentity{
		identifier:  identifier,
		title:       title,
		album:       album,
		artist:      artist,
		cleanTitle:  cleanTitle,
		cleanArtist: cleanArtist,
	}, true, nil
}

func appleMusicAffinityFlags(record map[string]any) (int, int, bool) {
	likeRating := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		jsonStringField(record, "Track Like Rating"),
		jsonStringField(record, "Preference"),
	)))
	if isFavorite, isDisliked, ok := appleMusicPreferenceFlags(likeRating); ok {
		return isFavorite, isDisliked, true
	}

	if jsonBoolField(record, "Favorite Status - Track") {
		return 1, 0, true
	}

	return 0, 0, false
}

func appleMusicPreferenceFlags(value string) (int, int, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "disliked", "dislike":
		return 0, 1, true
	case "liked", "like", "favorite", "favourite":
		return 1, 0, true
	default:
		return 0, 0, false
	}
}

func parseAppleMusicArtistTitleDescription(value string) (string, string, bool) {
	artist, title, found := strings.Cut(strings.TrimSpace(value), " - ")
	if !found {
		return "", "", false
	}
	artist = strings.TrimSpace(artist)
	title = strings.TrimSpace(title)
	if artist == "" || title == "" {
		return "", "", false
	}
	return artist, title, true
}

func jsonStringField(record map[string]any, names ...string) string {
	for _, name := range names {
		value, ok := record[name]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		case json.Number:
			return strings.TrimSpace(typed.String())
		case float64:
			return strings.TrimSpace(strconv.FormatFloat(typed, 'f', -1, 64))
		case bool:
			return strconv.FormatBool(typed)
		}
	}
	return ""
}

func jsonBoolField(record map[string]any, name string) bool {
	value, ok := record[name]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
}

func jsonIntField(record map[string]any, name string) (int, bool) {
	value, ok := record[name]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		return parsed, err == nil
	case float64:
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func boolFlag(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseOptionalInt64(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

func parseOptionalBoolFlag(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(strings.ToLower(value))
	if err != nil {
		return nil, err
	}
	return boolFlag(parsed), nil
}

func parseAppleMusicEpochMillis(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	millis, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return "", err
	}
	return time.UnixMilli(millis).UTC().Format(time.RFC3339Nano), nil
}

func nullableText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func isAmbientNoiseTelemetry(values ...string) bool {
	signatures := []string{
		"white noise",
		"bedtime mix",
		"rain sounds",
		"sleeping",
		"ocean waves",
		"radiance",
	}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		for _, signature := range signatures {
			if strings.Contains(value, signature) {
				return true
			}
		}
	}
	return false
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func refreshAlbumTrackCounts(tx *sql.Tx, albumCounts map[string]int) error {
	for albumID, parsedCount := range albumCounts {
		if strings.TrimSpace(albumID) == "" {
			continue
		}
		if _, err := tx.Exec(`
			UPDATE albums
			SET track_count = MAX(
				COALESCE(track_count, 0),
				?,
				(
					SELECT COUNT(DISTINCT id)
					FROM tracks
					WHERE album_id = ?
				)
			)
			WHERE id = ?`,
			parsedCount,
			albumID,
			albumID,
		); err != nil {
			return fmt.Errorf("failed to refresh track_count for album %q: %w", albumID, err)
		}
	}
	return nil
}
