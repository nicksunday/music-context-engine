package ingest

import (
	"path/filepath"
	"testing"

	"github.com/nicksunday/music-context-platform/internal/database"
)

func TestIngestAppleMusicLikesSelfHealsAndFavorites(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	tempDir := t.TempDir()
	db, err := database.InitDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	_, err = db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist)
		VALUES ('album-1', 'Café', 'Beyoncé', 'cafe', 'beyonce');
		INSERT INTO tracks (id, album_id, title, album, artist, clean_title, clean_artist, is_disliked)
		VALUES
			('track-1', 'album-1', 'Canción', 'Café', 'Beyoncé', 'cancion', 'beyonce', 1),
			('track-2', NULL, 'Other Song', 'Other Album', 'Other Artist', 'other song', 'other artist', 0);`)
	if err != nil {
		t.Fatalf("failed to insert fixtures: %v", err)
	}

	appleExport := filepath.Join(tempDir, "apple-music-likes.csv")
	writeTestCSV(t, appleExport, "Name,Artist,Composer,Album,Kind\r\" CANCIÓN \",\" Beyoncé \",\"Ignored\",\"Different Album\",\"AAC\"\r\"Missing Artist Track\",,\"Ignored\",,\"AAC\"\r\"New Track\",\"New Artist\",\"Ignored\",\"New Album\",\"AAC\"\r")

	result, err := IngestAppleMusicLikes(db, appleExport)
	if err != nil {
		t.Fatalf("IngestAppleMusicLikes() error = %v", err)
	}

	if result.Rows != 3 {
		t.Fatalf("Rows = %d, want 3", result.Rows)
	}
	if result.MatchedTracks != 1 {
		t.Fatalf("MatchedTracks = %d, want 1", result.MatchedTracks)
	}
	if result.CreatedTracks != 2 {
		t.Fatalf("CreatedTracks = %d, want 2", result.CreatedTracks)
	}
	if result.UnmatchedRows != 0 {
		t.Fatalf("UnmatchedRows = %d, want 0", result.UnmatchedRows)
	}

	var favorite int
	var disliked int
	var existingAlbumID string
	err = db.Ctx.QueryRow("SELECT album_id, is_favorite, is_disliked FROM tracks WHERE id = 'track-1'").Scan(&existingAlbumID, &favorite, &disliked)
	if err != nil {
		t.Fatalf("failed to query track-1 favorite flag: %v", err)
	}
	if existingAlbumID != "album-1" {
		t.Fatalf("track-1 album_id = %q, want %q", existingAlbumID, "album-1")
	}
	if favorite != 1 {
		t.Fatalf("track-1 is_favorite = %d, want 1", favorite)
	}
	if disliked != 0 {
		t.Fatalf("track-1 is_disliked = %d, want 0", disliked)
	}

	err = db.Ctx.QueryRow("SELECT is_favorite FROM tracks WHERE id = 'track-2'").Scan(&favorite)
	if err != nil {
		t.Fatalf("failed to query track-2 favorite flag: %v", err)
	}
	if favorite != 0 {
		t.Fatalf("track-2 is_favorite = %d, want 0", favorite)
	}

	var title, album, artist string
	var albumID string
	err = db.Ctx.QueryRow(
		"SELECT album_id, title, album, artist, is_favorite FROM tracks WHERE id = ?",
		trackID("New Track", "New Artist", "New Album"),
	).Scan(&albumID, &title, &album, &artist, &favorite)
	if err != nil {
		t.Fatalf("failed to query Apple Music-created track: %v", err)
	}
	if title != "New Track" {
		t.Fatalf("created title = %q, want %q", title, "New Track")
	}
	if album != "New Album" {
		t.Fatalf("created album = %q, want %q", album, "New Album")
	}
	if artist != "New Artist" {
		t.Fatalf("created artist = %q, want %q", artist, "New Artist")
	}
	if favorite != 1 {
		t.Fatalf("created is_favorite = %d, want 1", favorite)
	}

	var albumCount int
	err = db.Ctx.QueryRow(
		"SELECT COUNT(*) FROM albums WHERE id = ? AND title = 'New Album' AND artist = 'New Artist'",
		albumID,
	).Scan(&albumCount)
	if err != nil {
		t.Fatalf("failed to count New Album row: %v", err)
	}
	if albumCount != 1 {
		t.Fatalf("New Album count = %d, want 1", albumCount)
	}

	var unknownAlbumID string
	err = db.Ctx.QueryRow(
		"SELECT album_id, title, album, artist, is_favorite FROM tracks WHERE id = ?",
		trackID("Missing Artist Track", "Unknown Artist", "Unknown Album"),
	).Scan(&unknownAlbumID, &title, &album, &artist, &favorite)
	if err != nil {
		t.Fatalf("failed to query fallback Apple Music track: %v", err)
	}
	if title != "Missing Artist Track" {
		t.Fatalf("fallback title = %q, want %q", title, "Missing Artist Track")
	}
	if album != "Unknown Album" {
		t.Fatalf("fallback album = %q, want %q", album, "Unknown Album")
	}
	if artist != "Unknown Artist" {
		t.Fatalf("fallback artist = %q, want %q", artist, "Unknown Artist")
	}
	if favorite != 1 {
		t.Fatalf("fallback is_favorite = %d, want 1", favorite)
	}

	err = db.Ctx.QueryRow(
		"SELECT COUNT(*) FROM albums WHERE id = ? AND title = 'Unknown Album' AND artist = 'Unknown Artist'",
		unknownAlbumID,
	).Scan(&albumCount)
	if err != nil {
		t.Fatalf("failed to count Unknown Album row: %v", err)
	}
	if albumCount != 1 {
		t.Fatalf("Unknown Album count = %d, want 1", albumCount)
	}
}

func TestIngestAppleMusicLibraryTracksMapsAffinitiesAndTrackCounts(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	tempDir := t.TempDir()
	db, err := database.InitDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	_, err = db.Ctx.Exec(`
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist, is_favorite)
		VALUES ('existing-track', 'Known Song', 'Old Album', 'Known Artist', 'known song', 'known artist', 1);
	`)
	if err != nil {
		t.Fatalf("failed to insert fixture track: %v", err)
	}

	libraryPath := filepath.Join(tempDir, "Apple Music Library Tracks.json")
	writeTestCSV(t, libraryPath, `[
		{
			"Content Type": "Song",
			"Track Identifier": 1,
			"Title": "Known Song",
			"Artist": "Known Artist",
			"Album": "Known Album",
			"Track Like Rating": "DISLIKE"
		},
		{
			"Content Type": "Song",
			"Track Identifier": 2,
			"Title": "Theme",
			"Artist": "Composer",
			"Album": "Massive OST",
			"Track Count On Album": 40,
			"Favorite Status - Track": true
		},
		{
			"Content Type": "Song",
			"Track Identifier": 3,
			"Title": "Battle",
			"Artist": "Composer",
			"Album": "Massive OST",
			"Track Count On Album": 40,
			"Track Like Rating": "liked"
		}
	]`)

	result, err := IngestAppleMusicLibraryTracks(db, libraryPath)
	if err != nil {
		t.Fatalf("IngestAppleMusicLibraryTracks() error = %v", err)
	}
	if result.Rows != 3 {
		t.Fatalf("Rows = %d, want 3", result.Rows)
	}
	if result.MatchedTracks != 1 {
		t.Fatalf("MatchedTracks = %d, want 1", result.MatchedTracks)
	}
	if result.CreatedTracks != 2 {
		t.Fatalf("CreatedTracks = %d, want 2", result.CreatedTracks)
	}
	if result.PositiveRows != 2 {
		t.Fatalf("PositiveRows = %d, want 2", result.PositiveRows)
	}
	if result.DislikedRows != 1 {
		t.Fatalf("DislikedRows = %d, want 1", result.DislikedRows)
	}

	var favorite, disliked int
	var albumID string
	err = db.Ctx.QueryRow("SELECT album_id, is_favorite, is_disliked FROM tracks WHERE id = 'existing-track'").Scan(&albumID, &favorite, &disliked)
	if err != nil {
		t.Fatalf("failed to query updated existing track: %v", err)
	}
	if albumID == "" {
		t.Fatal("existing track album_id is empty")
	}
	if favorite != 0 || disliked != 1 {
		t.Fatalf("existing track flags = favorite %d disliked %d, want 0 and 1", favorite, disliked)
	}

	var trackCount int
	err = db.Ctx.QueryRow(
		"SELECT track_count FROM albums WHERE title = 'Massive OST' AND artist = 'Composer'",
	).Scan(&trackCount)
	if err != nil {
		t.Fatalf("failed to query Massive OST track_count: %v", err)
	}
	if trackCount != 40 {
		t.Fatalf("Massive OST track_count = %d, want 40", trackCount)
	}

	err = db.Ctx.QueryRow(
		"SELECT is_favorite, is_disliked FROM tracks WHERE id = ?",
		trackID("Theme", "Composer", "Massive OST"),
	).Scan(&favorite, &disliked)
	if err != nil {
		t.Fatalf("failed to query inserted favorite track: %v", err)
	}
	if favorite != 1 || disliked != 0 {
		t.Fatalf("inserted favorite flags = favorite %d disliked %d, want 1 and 0", favorite, disliked)
	}
}

func TestIngestAppleMusicFavoritesCSVMapsLikesAndDislikes(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	tempDir := t.TempDir()
	db, err := database.InitDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	_, err = db.Ctx.Exec(`
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist, is_favorite, is_disliked)
		VALUES
			('liked-track', 'Liked Song', 'Known Album', 'Known Artist', 'liked song', 'known artist', 0, 1),
			('disliked-track', 'Bad Song', 'Known Album', 'Known Artist', 'bad song', 'known artist', 1, 0);
	`)
	if err != nil {
		t.Fatalf("failed to insert fixture tracks: %v", err)
	}

	favoritesPath := filepath.Join(tempDir, "Apple Music - Favorites.csv")
	writeTestCSV(t, favoritesPath, `"Favorite Type","Item Reference","Item Description","Last Modified",Preference
Song,1,"Known Artist - Liked Song",2026-06-10T15:28:00Z,LIKE
Song,2,"Known Artist - Bad Song",2026-06-09T21:40:21Z,DISLIKE
Song,3,"New Artist - New Song",2026-06-09T19:58:11Z,LIKE
Album,4,"Known Artist - Known Album",2026-06-09T19:58:11Z,LIKE
`)

	result, err := IngestAppleMusicFavorites(db, favoritesPath)
	if err != nil {
		t.Fatalf("IngestAppleMusicFavorites() error = %v", err)
	}
	if result.Rows != 4 {
		t.Fatalf("Rows = %d, want 4", result.Rows)
	}
	if result.MatchedTracks != 2 {
		t.Fatalf("MatchedTracks = %d, want 2", result.MatchedTracks)
	}
	if result.CreatedTracks != 1 {
		t.Fatalf("CreatedTracks = %d, want 1", result.CreatedTracks)
	}
	if result.LikedRows != 2 {
		t.Fatalf("LikedRows = %d, want 2", result.LikedRows)
	}
	if result.DislikedRows != 1 {
		t.Fatalf("DislikedRows = %d, want 1", result.DislikedRows)
	}
	if result.UnmatchedRows != 1 {
		t.Fatalf("UnmatchedRows = %d, want 1", result.UnmatchedRows)
	}

	var favorite, disliked int
	err = db.Ctx.QueryRow("SELECT is_favorite, is_disliked FROM tracks WHERE id = 'liked-track'").Scan(&favorite, &disliked)
	if err != nil {
		t.Fatalf("failed to query liked track: %v", err)
	}
	if favorite != 1 || disliked != 0 {
		t.Fatalf("liked track flags = favorite %d disliked %d, want 1 and 0", favorite, disliked)
	}

	err = db.Ctx.QueryRow("SELECT is_favorite, is_disliked FROM tracks WHERE id = 'disliked-track'").Scan(&favorite, &disliked)
	if err != nil {
		t.Fatalf("failed to query disliked track: %v", err)
	}
	if favorite != 0 || disliked != 1 {
		t.Fatalf("disliked track flags = favorite %d disliked %d, want 0 and 1", favorite, disliked)
	}

	var album string
	err = db.Ctx.QueryRow(
		"SELECT album, is_favorite, is_disliked FROM tracks WHERE id = ?",
		trackID("New Song", "New Artist", "Unknown Album"),
	).Scan(&album, &favorite, &disliked)
	if err != nil {
		t.Fatalf("failed to query created favorite track: %v", err)
	}
	if album != "Unknown Album" || favorite != 1 || disliked != 0 {
		t.Fatalf("created track album=%q favorite=%d disliked=%d, want Unknown Album, 1, 0", album, favorite, disliked)
	}
}

func TestIngestAppleMusicLibraryActivityMapsNestedTracksAndIdentifierUpdates(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	tempDir := t.TempDir()
	db, err := database.InitDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	activityPath := filepath.Join(tempDir, "Apple Music Library Activity.json")
	writeTestCSV(t, activityPath, `[
		{
			"Transaction Type": "optInUser",
			"Transaction Identifier": 1,
			"Transaction Date": "2026-04-12T00:48:38Z"
		},
		{
			"Transaction Type": "addItems",
			"Transaction Identifier": 2,
			"Transaction Date": "2026-04-12T00:48:39Z",
			"Tracks": [
				{
					"Content Type": "Song",
					"Track Identifier": 10,
					"Title": "Known Song",
					"Artist": "Known Artist",
					"Album": "Known Album"
				}
			]
		},
		{
			"Transaction Type": "updateItems",
			"Transaction Identifier": 3,
			"Transaction Date": "2026-04-12T00:49:39Z",
			"Tracks": [
				{
					"Track Identifier": 10,
					"Track Like Rating": "DISLIKE",
					"Favorite Status - Track": false
				}
			]
		},
		{
			"Transaction Type": "updateItems",
			"Transaction Identifier": 4,
			"Transaction Date": "2026-04-12T00:50:39Z",
			"Tracks": [
				{
					"Track Identifier": 99,
					"Track Like Rating": "liked"
				}
			]
		}
	]`)

	result, err := IngestAppleMusicLibraryActivity(db, activityPath)
	if err != nil {
		t.Fatalf("IngestAppleMusicLibraryActivity() error = %v", err)
	}
	if result.Rows != 4 {
		t.Fatalf("Rows = %d, want 4", result.Rows)
	}
	if result.TrackRows != 3 {
		t.Fatalf("TrackRows = %d, want 3", result.TrackRows)
	}
	if result.CreatedTracks != 1 {
		t.Fatalf("CreatedTracks = %d, want 1", result.CreatedTracks)
	}
	if result.MatchedTracks != 1 {
		t.Fatalf("MatchedTracks = %d, want 1", result.MatchedTracks)
	}
	if result.DislikedRows != 1 {
		t.Fatalf("DislikedRows = %d, want 1", result.DislikedRows)
	}
	if result.UnmatchedTrackRows != 1 {
		t.Fatalf("UnmatchedTrackRows = %d, want 1", result.UnmatchedTrackRows)
	}
	if result.AlbumsUpdated != 1 {
		t.Fatalf("AlbumsUpdated = %d, want 1", result.AlbumsUpdated)
	}

	var albumID string
	var favorite, disliked int
	err = db.Ctx.QueryRow(
		"SELECT album_id, is_favorite, is_disliked FROM tracks WHERE id = ?",
		trackID("Known Song", "Known Artist", "Known Album"),
	).Scan(&albumID, &favorite, &disliked)
	if err != nil {
		t.Fatalf("failed to query library activity track: %v", err)
	}
	if albumID == "" {
		t.Fatal("albumID is empty")
	}
	if favorite != 0 || disliked != 1 {
		t.Fatalf("track flags = favorite %d disliked %d, want 0 and 1", favorite, disliked)
	}

	var trackCount int
	err = db.Ctx.QueryRow("SELECT track_count FROM albums WHERE id = ?", albumID).Scan(&trackCount)
	if err != nil {
		t.Fatalf("failed to query album track_count: %v", err)
	}
	if trackCount != 1 {
		t.Fatalf("trackCount = %d, want 1", trackCount)
	}
}

func TestIngestAppleMusicPlayActivityStoresEndReasonAndSkipsAmbientRows(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	tempDir := t.TempDir()
	db, err := database.InitDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	activityPath := filepath.Join(tempDir, "Apple Music Play Activity.csv")
	writeTestCSV(t, activityPath, `"Event Timestamp","Track Description","Song Name","Play Duration Milliseconds","End Reason Type","Container Name","Artist","Event ID"
"2026-06-01T00:00:00Z","Active Song","Active Song","120000","MANUAL_SKIP","Focus","Active Artist","event-1"
"2026-06-01T00:01:00Z","Rain Loop","Rain Loop","60000","NATURAL_END_OF_TRACK","white noise focus","Ambient Artist","event-2"
"2026-06-01T00:02:00Z","Finished Song","Finished Song","180000","NATURAL_END_OF_TRACK","Library","Active Artist","event-3"
`)

	result, err := IngestAppleMusicPlayActivity(db, activityPath)
	if err != nil {
		t.Fatalf("IngestAppleMusicPlayActivity() error = %v", err)
	}
	if result.Rows != 3 {
		t.Fatalf("Rows = %d, want 3", result.Rows)
	}
	if result.SkippedAmbientRows != 1 {
		t.Fatalf("SkippedAmbientRows = %d, want 1", result.SkippedAmbientRows)
	}
	if result.WrittenRows != 2 {
		t.Fatalf("WrittenRows = %d, want 2", result.WrittenRows)
	}

	var storedRows int
	err = db.Ctx.QueryRow("SELECT COUNT(*) FROM apple_music_play_activity").Scan(&storedRows)
	if err != nil {
		t.Fatalf("failed to count play activity rows: %v", err)
	}
	if storedRows != 2 {
		t.Fatalf("storedRows = %d, want 2", storedRows)
	}

	var endReason string
	var wasSkipped int
	err = db.Ctx.QueryRow("SELECT end_reason_type, was_skipped FROM apple_music_play_activity WHERE id = 'event-1'").Scan(&endReason, &wasSkipped)
	if err != nil {
		t.Fatalf("failed to query skipped play activity row: %v", err)
	}
	if endReason != "MANUAL_SKIP" || wasSkipped != 1 {
		t.Fatalf("event-1 end_reason_type=%q was_skipped=%d, want MANUAL_SKIP and 1", endReason, wasSkipped)
	}
}

func TestIngestAppleMusicTrackPlayHistoryStoresCompactTelemetry(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	tempDir := t.TempDir()
	db, err := database.InitDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	historyPath := filepath.Join(tempDir, "Apple Music - Track Play History.csv")
	writeTestCSV(t, historyPath, `"Track Name","Last Played Date","Is User Initiated"
"Lamb of God - 512",1631928545385,false
"Dido - Thank You",1631928545386,true
"Rain Sounds - Ocean Waves",1631928545387,true
"Malformed Track",1631928545388,false
`)

	result, err := IngestAppleMusicTrackPlayHistory(db, historyPath)
	if err != nil {
		t.Fatalf("IngestAppleMusicTrackPlayHistory() error = %v", err)
	}
	if result.Rows != 4 {
		t.Fatalf("Rows = %d, want 4", result.Rows)
	}
	if result.WrittenRows != 2 {
		t.Fatalf("WrittenRows = %d, want 2", result.WrittenRows)
	}
	if result.SkippedAmbientRows != 1 {
		t.Fatalf("SkippedAmbientRows = %d, want 1", result.SkippedAmbientRows)
	}
	if result.UnmatchedRows != 1 {
		t.Fatalf("UnmatchedRows = %d, want 1", result.UnmatchedRows)
	}

	var eventTimestamp, songName, artist, endReason, sourceType string
	var wasSkipped, userInitiated int
	err = db.Ctx.QueryRow(`
		SELECT event_timestamp, song_name, artist, end_reason_type, was_skipped, source_type, is_user_initiated
		FROM apple_music_play_activity
		WHERE track_description = 'Dido - Thank You'`,
	).Scan(&eventTimestamp, &songName, &artist, &endReason, &wasSkipped, &sourceType, &userInitiated)
	if err != nil {
		t.Fatalf("failed to query compact play history row: %v", err)
	}
	if eventTimestamp != "2021-09-18T01:29:05.386Z" {
		t.Fatalf("eventTimestamp = %q, want 2021-09-18T01:29:05.386Z", eventTimestamp)
	}
	if songName != "Thank You" || artist != "Dido" {
		t.Fatalf("song/artist = %q/%q, want Thank You/Dido", songName, artist)
	}
	if endReason != "TRACK_PLAY_HISTORY" || wasSkipped != 0 {
		t.Fatalf("endReason=%q wasSkipped=%d, want TRACK_PLAY_HISTORY and 0", endReason, wasSkipped)
	}
	if sourceType != "track_play_history" || userInitiated != 1 {
		t.Fatalf("sourceType=%q userInitiated=%d, want track_play_history and 1", sourceType, userInitiated)
	}
}

func TestIngestAppleMusicLikesAcceptsBareQuotes(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	tempDir := t.TempDir()
	db, err := database.InitDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	appleExport := filepath.Join(tempDir, "apple-music-likes.csv")
	writeTestCSV(t, appleExport, "Name,Artist,Composer,Album,Kind\r\"Loca (Remasterizado)\",Osmani Garcia \"La Voz\",Ignored,El Malcriao,AAC\r")

	result, err := IngestAppleMusicLikes(db, appleExport)
	if err != nil {
		t.Fatalf("IngestAppleMusicLikes() error = %v", err)
	}
	if result.CreatedTracks != 1 {
		t.Fatalf("CreatedTracks = %d, want 1", result.CreatedTracks)
	}

	var artist string
	err = db.Ctx.QueryRow(
		"SELECT artist FROM tracks WHERE id = ?",
		trackID("Loca (Remasterizado)", "Osmani Garcia \"La Voz\"", "El Malcriao"),
	).Scan(&artist)
	if err != nil {
		t.Fatalf("failed to query Apple Music track: %v", err)
	}
	if artist != "Osmani Garcia \"La Voz\"" {
		t.Fatalf("artist = %q, want %q", artist, "Osmani Garcia \"La Voz\"")
	}
}
