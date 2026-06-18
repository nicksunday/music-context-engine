package database

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestReconcileDuplicateAlbumsKeepsHighestRatingAndRewritesTracks(t *testing.T) {
	db := openOptimizerTestDB(t)

	insertAlbumForReconciliation(t, db, "album-old", "same album", "same artist", nil, nil)
	insertAlbumForReconciliation(t, db, "album-rated-low", "same album", "same artist", `["metal"]`, 3.5)
	insertAlbumForReconciliation(t, db, "album-rated-high", "same album", "same artist", nil, 4.5)
	insertTrackForReconciliation(t, db, "track-old", "album-old")
	insertTrackForReconciliation(t, db, "track-low", "album-rated-low")
	insertTrackForReconciliation(t, db, "track-high", "album-rated-high")

	purgedRows, err := ReconcileDuplicateAlbums(db)
	if err != nil {
		t.Fatalf("ReconcileDuplicateAlbums() error = %v", err)
	}
	if purgedRows != 2 {
		t.Fatalf("purgedRows = %d, want 2", purgedRows)
	}

	assertAlbumIDsForCleanKey(t, db, "same album", "same artist", []string{"album-rated-high"})
	assertTrackAlbumID(t, db, "track-old", "album-rated-high")
	assertTrackAlbumID(t, db, "track-low", "album-rated-high")
	assertTrackAlbumID(t, db, "track-high", "album-rated-high")
}

func TestReconcileDuplicateAlbumsKeepsPopulatedGenresWhenRatingsAreMissing(t *testing.T) {
	db := openOptimizerTestDB(t)

	insertAlbumForReconciliation(t, db, "album-a-oldest", "genre album", "genre artist", nil, nil)
	insertAlbumForReconciliation(t, db, "album-z-genres", "genre album", "genre artist", `["ambient"]`, nil)
	insertTrackForReconciliation(t, db, "track-genre", "album-a-oldest")

	purgedRows, err := ReconcileDuplicateAlbums(db)
	if err != nil {
		t.Fatalf("ReconcileDuplicateAlbums() error = %v", err)
	}
	if purgedRows != 1 {
		t.Fatalf("purgedRows = %d, want 1", purgedRows)
	}

	assertAlbumIDsForCleanKey(t, db, "genre album", "genre artist", []string{"album-z-genres"})
	assertTrackAlbumID(t, db, "track-genre", "album-z-genres")
}

func TestReconcileDuplicateAlbumsFallsBackToOldestID(t *testing.T) {
	db := openOptimizerTestDB(t)

	insertAlbumForReconciliation(t, db, "album-b", "plain album", "plain artist", "not-json", nil)
	insertAlbumForReconciliation(t, db, "album-a", "plain album", "plain artist", "[]", nil)
	insertAlbumForReconciliation(t, db, "album-c", "plain album", "plain artist", nil, nil)
	insertTrackForReconciliation(t, db, "track-plain", "album-c")

	purgedRows, err := ReconcileDuplicateAlbums(db)
	if err != nil {
		t.Fatalf("ReconcileDuplicateAlbums() error = %v", err)
	}
	if purgedRows != 2 {
		t.Fatalf("purgedRows = %d, want 2", purgedRows)
	}

	assertAlbumIDsForCleanKey(t, db, "plain album", "plain artist", []string{"album-a"})
	assertTrackAlbumID(t, db, "track-plain", "album-a")
}

func TestReconcileDuplicateAlbumsNoopsWhenNoDuplicateCleanKeysExist(t *testing.T) {
	db := openOptimizerTestDB(t)

	insertAlbumForReconciliation(t, db, "album-1", "first album", "artist", nil, nil)
	insertAlbumForReconciliation(t, db, "album-2", "second album", "artist", nil, nil)
	insertTrackForReconciliation(t, db, "track-1", "album-1")

	purgedRows, err := ReconcileDuplicateAlbums(db)
	if err != nil {
		t.Fatalf("ReconcileDuplicateAlbums() error = %v", err)
	}
	if purgedRows != 0 {
		t.Fatalf("purgedRows = %d, want 0", purgedRows)
	}

	assertAlbumIDsForCleanKey(t, db, "first album", "artist", []string{"album-1"})
	assertAlbumIDsForCleanKey(t, db, "second album", "artist", []string{"album-2"})
	assertTrackAlbumID(t, db, "track-1", "album-1")
}

func openOptimizerTestDB(t *testing.T) *DBClient {
	t.Helper()

	unsetMusicVaultDBPathEnv(t)

	db, err := InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Ctx.Close(); err != nil {
			t.Fatalf("failed to close test database: %v", err)
		}
	})

	return db
}

func insertAlbumForReconciliation(t *testing.T, db *DBClient, id, cleanTitle, cleanArtist string, genres any, rating any) {
	t.Helper()

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres, user_rating)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id,
		"Title "+id,
		"Artist "+id,
		cleanTitle,
		cleanArtist,
		genres,
		rating,
	)
	if err != nil {
		t.Fatalf("failed to insert album %q: %v", id, err)
	}
}

func insertTrackForReconciliation(t *testing.T, db *DBClient, id, albumID string) {
	t.Helper()

	_, err := db.Ctx.Exec(`
		INSERT INTO tracks (id, album_id, title, album, artist, clean_title, clean_artist)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id,
		albumID,
		"Track "+id,
		"Album "+albumID,
		"Artist",
		"track "+id,
		"artist",
	)
	if err != nil {
		t.Fatalf("failed to insert track %q: %v", id, err)
	}
}

func assertAlbumIDsForCleanKey(t *testing.T, db *DBClient, cleanTitle, cleanArtist string, want []string) {
	t.Helper()

	rows, err := db.Ctx.Query(
		"SELECT id FROM albums WHERE clean_title = ? AND clean_artist = ? ORDER BY id",
		cleanTitle,
		cleanArtist,
	)
	if err != nil {
		t.Fatalf("failed to query albums for clean key: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("failed to scan album id: %v", err)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("failed to iterate album ids: %v", err)
	}

	if !slices.Equal(got, want) {
		t.Fatalf("album ids for (%q, %q) = %v, want %v", cleanTitle, cleanArtist, got, want)
	}
}

func assertTrackAlbumID(t *testing.T, db *DBClient, trackID, want string) {
	t.Helper()

	var got string
	err := db.Ctx.QueryRow("SELECT album_id FROM tracks WHERE id = ?", trackID).Scan(&got)
	if err != nil {
		t.Fatalf("failed to query track %q album_id: %v", trackID, err)
	}
	if got != want {
		t.Fatalf("track %q album_id = %q, want %q", trackID, got, want)
	}
}
