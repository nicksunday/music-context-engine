package ingest

import (
	"path/filepath"
	"testing"

	"github.com/nicksunday/music-context-platform/internal/database"
)

func TestIngestLastFMFavoritesSelfHealsMissingTracks(t *testing.T) {
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
		INSERT INTO tracks (id, album_id, title, album, artist, clean_title, clean_artist)
		VALUES
			('track-1', 'album-1', 'Canción', 'Café', 'Beyoncé', 'cancion', 'beyonce'),
			('track-2', NULL, 'Other Song', 'Other Album', 'Other Artist', 'other song', 'other artist');`)
	if err != nil {
		t.Fatalf("failed to insert fixtures: %v", err)
	}

	lastFMExport := filepath.Join(tempDir, "lastfm.csv")
	writeTestCSV(t, lastFMExport, `uts,utc_time,artist,artist_mbid,album,album_mbid,track,track_mbid
"1","01 Jan 2020, 00:00"," Beyoncé ","","",""," CANCIÓN ","mbid-1"
"2","01 Jan 2020, 00:01","Missing Artist","","","","Missing Track","mbid-2"
"3","01 Jan 2020, 00:02","New Artist","","New Album","","New Track","mbid-3"
`)

	result, err := IngestLastFMFavorites(db, lastFMExport, nil)
	if err != nil {
		t.Fatalf("IngestLastFMFavorites() error = %v", err)
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
	var existingAlbumID string
	err = db.Ctx.QueryRow("SELECT album_id, is_favorite FROM tracks WHERE id = 'track-1'").Scan(&existingAlbumID, &favorite)
	if err != nil {
		t.Fatalf("failed to query track-1 favorite flag: %v", err)
	}
	if existingAlbumID != "album-1" {
		t.Fatalf("track-1 album_id = %q, want %q", existingAlbumID, "album-1")
	}
	if favorite != 1 {
		t.Fatalf("track-1 is_favorite = %d, want 1", favorite)
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
		t.Fatalf("failed to query Last.fm-created track: %v", err)
	}
	if albumID == "" {
		t.Fatal("created track album_id is empty")
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
		trackID("Missing Track", "Missing Artist", "Unknown Album"),
	).Scan(&unknownAlbumID, &title, &album, &artist, &favorite)
	if err != nil {
		t.Fatalf("failed to query Unknown Album-created track: %v", err)
	}
	if title != "Missing Track" {
		t.Fatalf("unknown-album title = %q, want %q", title, "Missing Track")
	}
	if album != "Unknown Album" {
		t.Fatalf("unknown-album album = %q, want %q", album, "Unknown Album")
	}
	if artist != "Missing Artist" {
		t.Fatalf("unknown-album artist = %q, want %q", artist, "Missing Artist")
	}
	if favorite != 1 {
		t.Fatalf("unknown-album is_favorite = %d, want 1", favorite)
	}

	err = db.Ctx.QueryRow(
		"SELECT COUNT(*) FROM albums WHERE id = ? AND title = 'Unknown Album' AND artist = 'Missing Artist'",
		unknownAlbumID,
	).Scan(&albumCount)
	if err != nil {
		t.Fatalf("failed to count Unknown Album row: %v", err)
	}
	if albumCount != 1 {
		t.Fatalf("Unknown Album count = %d, want 1", albumCount)
	}
}
