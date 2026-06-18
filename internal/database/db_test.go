package database

import (
	"context"
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDatabasePathFallsBackToDefaultPath(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	defaultPath := filepath.Join(t.TempDir(), "default.db")
	got, err := ResolveDatabasePath(defaultPath)
	if err != nil {
		t.Fatalf("ResolveDatabasePath() error = %v", err)
	}

	if got != defaultPath {
		t.Fatalf("ResolveDatabasePath() = %q, want %q", got, defaultPath)
	}
}

func TestResolveDatabasePathFallsBackToDataDirectory(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	got, err := ResolveDatabasePath("")
	if err != nil {
		t.Fatalf("ResolveDatabasePath() error = %v", err)
	}

	want := filepath.Join("data", "music_vault.db")
	if got != want {
		t.Fatalf("ResolveDatabasePath() = %q, want %q", got, want)
	}
}

func TestResolveDatabasePathTreatsBlankEnvironmentPathAsUnset(t *testing.T) {
	defaultPath := filepath.Join(t.TempDir(), "default.db")
	t.Setenv(MusicVaultDBPathEnv, "  ")

	got, err := ResolveDatabasePath(defaultPath)
	if err != nil {
		t.Fatalf("ResolveDatabasePath() error = %v", err)
	}

	if got != defaultPath {
		t.Fatalf("ResolveDatabasePath() = %q, want %q", got, defaultPath)
	}
}

func TestResolveDatabasePathUsesAbsoluteEnvironmentPath(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), "env.db")
	t.Setenv(MusicVaultDBPathEnv, envPath)

	got, err := ResolveDatabasePath(filepath.Join(t.TempDir(), "default.db"))
	if err != nil {
		t.Fatalf("ResolveDatabasePath() error = %v", err)
	}

	if got != envPath {
		t.Fatalf("ResolveDatabasePath() = %q, want %q", got, envPath)
	}
}

func TestResolveDatabasePathExpandsHomeEnvironmentPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}
	t.Setenv(MusicVaultDBPathEnv, "~/music-vault-test.db")

	got, err := ResolveDatabasePath(filepath.Join(t.TempDir(), "default.db"))
	if err != nil {
		t.Fatalf("ResolveDatabasePath() error = %v", err)
	}

	want := filepath.Join(homeDir, "music-vault-test.db")
	if got != want {
		t.Fatalf("ResolveDatabasePath() = %q, want %q", got, want)
	}
}

func TestResolveDatabasePathRejectsRelativeEnvironmentPath(t *testing.T) {
	t.Setenv(MusicVaultDBPathEnv, "music_vault.db")

	if _, err := ResolveDatabasePath(filepath.Join(t.TempDir(), "default.db")); err == nil {
		t.Fatal("ResolveDatabasePath() error = nil, want error")
	}
}

func TestInitDBAtPathIgnoresEnvironmentPath(t *testing.T) {
	t.Setenv(MusicVaultDBPathEnv, "music_vault.db")

	dbPath := filepath.Join(t.TempDir(), "explicit.db")
	db, err := InitDBAtPath(dbPath)
	if err != nil {
		t.Fatalf("InitDBAtPath() error = %v", err)
	}
	defer db.Ctx.Close()

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file was not created at %q: %v", dbPath, err)
	}
}

func TestInitDBCreatesParentDirectory(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	dbPath := filepath.Join(t.TempDir(), "nested", "music_vault.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file was not created at %q: %v", dbPath, err)
	}
}

func TestInitDBAddsAndBackfillsCleanColumns(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	dbPath := filepath.Join(t.TempDir(), "test.db")

	oldDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	_, err = oldDB.Exec(`
		CREATE TABLE albums (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			artist TEXT NOT NULL,
			rym_rating REAL DEFAULT NULL,
			release_date INTEGER
		);
		CREATE TABLE tracks (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			album TEXT,
			artist TEXT NOT NULL,
			source TEXT NOT NULL,
			rating REAL DEFAULT 0.0
		);
		INSERT INTO albums (id, title, artist, rym_rating, release_date) VALUES ('album-1', 'Crème Brûlée', 'François Hardy', 4.5, 1969);
		INSERT INTO tracks (id, title, album, artist, source) VALUES ('track-1', 'Canción', 'Café', 'Beyoncé', 'test');
		CREATE VIEW v_artist_affinity AS SELECT id FROM albums;
		CREATE VIEW v_genre_topography AS SELECT id FROM tracks;
	`)
	if err != nil {
		t.Fatalf("failed to create old schema: %v", err)
	}
	if err := oldDB.Close(); err != nil {
		t.Fatalf("failed to close old db: %v", err)
	}

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	var albumTitle, albumArtist string
	err = db.Ctx.QueryRow(
		"SELECT clean_title, clean_artist FROM albums WHERE id = 'album-1'",
	).Scan(&albumTitle, &albumArtist)
	if err != nil {
		t.Fatalf("failed to query album clean columns: %v", err)
	}
	if albumTitle != "creme brulee" {
		t.Fatalf("albumTitle = %q, want %q", albumTitle, "creme brulee")
	}
	if albumArtist != "francois hardy" {
		t.Fatalf("albumArtist = %q, want %q", albumArtist, "francois hardy")
	}

	var trackTitle, trackArtist string
	err = db.Ctx.QueryRow(
		"SELECT clean_title, clean_artist FROM tracks WHERE id = 'track-1'",
	).Scan(&trackTitle, &trackArtist)
	if err != nil {
		t.Fatalf("failed to query track clean columns: %v", err)
	}
	if trackTitle != "cancion" {
		t.Fatalf("trackTitle = %q, want %q", trackTitle, "cancion")
	}
	if trackArtist != "beyonce" {
		t.Fatalf("trackArtist = %q, want %q", trackArtist, "beyonce")
	}

	var userRating float64
	var releaseDate int
	err = db.Ctx.QueryRow(
		"SELECT user_rating, release_date FROM albums WHERE id = 'album-1'",
	).Scan(&userRating, &releaseDate)
	if err != nil {
		t.Fatalf("failed to query migrated album valuation columns: %v", err)
	}
	if userRating != 4.5 {
		t.Fatalf("userRating = %v, want 4.5", userRating)
	}
	if releaseDate != 1969 {
		t.Fatalf("releaseDate = %v, want 1969", releaseDate)
	}

	for _, table := range []string{"albums", "tracks"} {
		exists, err := columnExists(db.Ctx, table, "genres")
		if err != nil {
			t.Fatalf("columnExists(%q, genres) error = %v", table, err)
		}
		if !exists {
			t.Fatalf("%s.genres column was not added", table)
		}
	}

	exists, err := columnExists(db.Ctx, "albums", "track_count")
	if err != nil {
		t.Fatalf("columnExists(albums, track_count) error = %v", err)
	}
	if !exists {
		t.Fatal("albums.track_count column was not added")
	}

	for _, column := range []string{"source", "rating"} {
		exists, err = columnExists(db.Ctx, "tracks", column)
		if err != nil {
			t.Fatalf("columnExists(tracks, %s) error = %v", column, err)
		}
		if exists {
			t.Fatalf("tracks.%s legacy column still exists", column)
		}
	}

	exists, err = columnExists(db.Ctx, "tracks", "is_favorite")
	if err != nil {
		t.Fatalf("columnExists(tracks, is_favorite) error = %v", err)
	}
	if !exists {
		t.Fatal("tracks.is_favorite column was not added")
	}

	exists, err = columnExists(db.Ctx, "tracks", "is_disliked")
	if err != nil {
		t.Fatalf("columnExists(tracks, is_disliked) error = %v", err)
	}
	if !exists {
		t.Fatal("tracks.is_disliked column was not added")
	}

	var indexCount int
	err = db.Ctx.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_tracks_is_favorite'",
	).Scan(&indexCount)
	if err != nil {
		t.Fatalf("failed to query is_favorite index: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("idx_tracks_is_favorite count = %d, want 1", indexCount)
	}

	err = db.Ctx.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_tracks_is_disliked'",
	).Scan(&indexCount)
	if err != nil {
		t.Fatalf("failed to query is_disliked index: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("idx_tracks_is_disliked count = %d, want 1", indexCount)
	}

	exists, err = columnExists(db.Ctx, "tracks", "album_id")
	if err != nil {
		t.Fatalf("columnExists(tracks, album_id) error = %v", err)
	}
	if !exists {
		t.Fatal("tracks.album_id column was not added")
	}

	exists, err = columnExists(db.Ctx, "albums", "rym_rating")
	if err != nil {
		t.Fatalf("columnExists(albums, rym_rating) error = %v", err)
	}
	if exists {
		t.Fatal("albums.rym_rating legacy column still exists")
	}

	exists, err = columnExists(db.Ctx, "albums", "user_rating")
	if err != nil {
		t.Fatalf("columnExists(albums, user_rating) error = %v", err)
	}
	if !exists {
		t.Fatal("albums.user_rating column was not added")
	}
}

func TestInitDBRefreshesExistingCleanColumns(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	dbPath := filepath.Join(t.TempDir(), "refresh.db")

	staleDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	_, err = staleDB.Exec(`
		CREATE TABLE albums (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			artist TEXT NOT NULL,
			clean_title TEXT NOT NULL,
			clean_artist TEXT NOT NULL,
			genres TEXT,
			user_rating REAL,
			release_date INTEGER
		);
		CREATE TABLE tracks (
			id TEXT PRIMARY KEY,
			album_id TEXT REFERENCES albums(id),
			title TEXT NOT NULL,
			album TEXT,
			artist TEXT NOT NULL,
			clean_title TEXT NOT NULL,
			clean_artist TEXT NOT NULL,
			genres TEXT,
			is_favorite INTEGER DEFAULT 0
		);
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, user_rating)
		VALUES ('album-1', 'Polygondwanaland', 'King Gizzard and The Lizard Wizard', 'polygondwanaland', 'king gizzard and the lizard wizard', 4.5);
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist, is_favorite)
		VALUES ('track-1', 'Crumbling Castle', 'Polygondwanaland', 'King Gizzard & The Lizard Wizard', 'stale title', 'king gizzard & the lizard wizard', 1);
	`)
	if err != nil {
		t.Fatalf("failed to create stale schema: %v", err)
	}
	if err := staleDB.Close(); err != nil {
		t.Fatalf("failed to close stale db: %v", err)
	}

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	var cleanTitle, cleanArtist string
	err = db.Ctx.QueryRow(
		"SELECT clean_title, clean_artist FROM tracks WHERE id = 'track-1'",
	).Scan(&cleanTitle, &cleanArtist)
	if err != nil {
		t.Fatalf("failed to query refreshed track clean columns: %v", err)
	}
	if cleanTitle != "crumbling castle" {
		t.Fatalf("cleanTitle = %q, want %q", cleanTitle, "crumbling castle")
	}
	if cleanArtist != "king gizzard and the lizard wizard" {
		t.Fatalf("cleanArtist = %q, want %q", cleanArtist, "king gizzard and the lizard wizard")
	}

	var avgRating sql.NullFloat64
	err = db.Ctx.QueryRow(`
		SELECT avg_user_rating
		FROM v_artist_affinity
		WHERE clean_artist = 'king gizzard and the lizard wizard'`,
	).Scan(&avgRating)
	if err != nil {
		t.Fatalf("failed to query King Gizzard affinity: %v", err)
	}
	if !avgRating.Valid || !floatClose(avgRating.Float64, 4.5) {
		t.Fatalf("avgRating = %#v, want 4.5", avgRating)
	}
}

func TestArtistAffinityViewScoresAndOrdersExplicitSignals(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	db, err := InitDB(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	_, err = db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, user_rating, track_count)
		VALUES
			('album-1', 'Crack the Skye', 'Mastodon', 'crack the skye', 'mastodon', 4.5, 7),
			('album-2', 'Heaven or Las Vegas', 'Cocteau Twins', 'heaven or las vegas', 'cocteau twins', 5.0, 10),
			('album-3', 'Unrated', 'Mastodon', 'unrated', 'mastodon', NULL, 9);
		INSERT INTO tracks (id, album_id, title, album, artist, clean_title, clean_artist, is_favorite)
		VALUES
			('track-1', 'album-1', 'Oblivion', 'Crack the Skye', 'Mastodon', 'oblivion', 'mastodon', 1),
			('track-2', NULL, 'Blood and Thunder', 'Leviathan', 'Mastodon', 'blood and thunder', 'mastodon', 1),
			('track-3', 'album-2', 'Cherry-coloured Funk', 'Heaven or Las Vegas', 'Cocteau Twins', 'cherry coloured funk', 'cocteau twins', 0),
			('track-4', NULL, 'Favorite Without Rating', 'Single', 'No Score Artist', 'favorite without rating', 'no score artist', 1),
			('track-5', NULL, 'No Explicit Signal', 'Single', 'Unranked Artist', 'no explicit signal', 'unranked artist', 0);
	`)
	if err != nil {
		t.Fatalf("failed to insert artist affinity fixtures: %v", err)
	}

	rows, err := db.Ctx.Query(`
		SELECT artist, clean_artist, favorite_tracks_count, disliked_tracks_count, avg_user_rating, curved_affinity_score
		FROM v_artist_affinity`)
	if err != nil {
		t.Fatalf("failed to query v_artist_affinity: %v", err)
	}
	defer rows.Close()

	var got []ArtistAffinity
	for rows.Next() {
		var row ArtistAffinity
		if err := rows.Scan(&row.Artist, &row.CleanArtist, &row.FavoriteTracksCount, &row.DislikedTracksCount, &row.AvgUserRating, &row.CurvedAffinityScore); err != nil {
			t.Fatalf("failed to scan affinity row: %v", err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("affinity rows error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3; rows = %#v", len(got), got)
	}
	if got[0].CleanArtist != "cocteau twins" {
		t.Fatalf("got[0].CleanArtist = %q, want cocteau twins", got[0].CleanArtist)
	}
	if !got[0].AvgUserRating.Valid || !floatClose(got[0].AvgUserRating.Float64, 5.0) {
		t.Fatalf("cocteau twins avg rating = %#v, want 5.0", got[0].AvgUserRating)
	}
	if !floatClose(got[0].CurvedAffinityScore, 10.0) {
		t.Fatalf("cocteau twins curved affinity score = %v, want 10", got[0].CurvedAffinityScore)
	}
	if got[1].CleanArtist != "mastodon" {
		t.Fatalf("got[1].CleanArtist = %q, want mastodon", got[1].CleanArtist)
	}
	if got[1].FavoriteTracksCount != 2 {
		t.Fatalf("mastodon favorite count = %d, want 2", got[1].FavoriteTracksCount)
	}
	if got[1].DislikedTracksCount != 0 {
		t.Fatalf("mastodon disliked count = %d, want 0", got[1].DislikedTracksCount)
	}
	if !got[1].AvgUserRating.Valid || !floatClose(got[1].AvgUserRating.Float64, 4.5) {
		t.Fatalf("mastodon avg rating = %#v, want 4.5", got[1].AvgUserRating)
	}
	if !floatClose(got[1].CurvedAffinityScore, 9.0) {
		t.Fatalf("mastodon curved affinity score = %v, want 9", got[1].CurvedAffinityScore)
	}
	if got[2].CleanArtist != "no score artist" {
		t.Fatalf("got[2].CleanArtist = %q, want no score artist", got[2].CleanArtist)
	}
	if got[2].AvgUserRating.Valid {
		t.Fatalf("no score artist avg rating = %#v, want NULL", got[2].AvgUserRating)
	}
	if !floatClose(got[2].CurvedAffinityScore, 1.0) {
		t.Fatalf("no score artist curved affinity score = %v, want 1", got[2].CurvedAffinityScore)
	}

	affinities, err := FetchTopArtistAffinities(context.Background(), db.Ctx, 2)
	if err != nil {
		t.Fatalf("FetchTopArtistAffinities() error = %v", err)
	}
	if len(affinities) != 2 || affinities[0].CleanArtist != "cocteau twins" || affinities[1].CleanArtist != "mastodon" {
		t.Fatalf("FetchTopArtistAffinities() = %#v, want cocteau twins then mastodon", affinities)
	}
}

func TestArtistAffinityViewClampsOutliersAndPenalizesDislikes(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	db, err := InitDB(filepath.Join(t.TempDir(), "analytics-clamp.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	_, err = db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, user_rating, track_count)
		VALUES ('album-1', 'Massive Soundtrack', 'Outlier Artist', 'massive soundtrack', 'outlier artist', 5.0, 120);
		INSERT INTO tracks (id, album_id, title, album, artist, clean_title, clean_artist, is_favorite, is_disliked)
		VALUES
			('track-1', 'album-1', 'Theme', 'Massive Soundtrack', 'Outlier Artist', 'theme', 'outlier artist', 1, 0),
			('track-2', 'album-1', 'Interlude', 'Massive Soundtrack', 'Outlier Artist', 'interlude', 'outlier artist', 1, 0),
			('track-3', 'album-1', 'Bad Loop', 'Massive Soundtrack', 'Outlier Artist', 'bad loop', 'outlier artist', 0, 1),
			('track-4', NULL, 'Rejected Single', 'Single', 'Outlier Artist', 'rejected single', 'outlier artist', 0, 1);
	`)
	if err != nil {
		t.Fatalf("failed to insert clamped affinity fixtures: %v", err)
	}

	var row ArtistAffinity
	err = db.Ctx.QueryRow(`
		SELECT artist, clean_artist, favorite_tracks_count, disliked_tracks_count, avg_user_rating, curved_affinity_score
		FROM v_artist_affinity
		WHERE clean_artist = 'outlier artist'`,
	).Scan(&row.Artist, &row.CleanArtist, &row.FavoriteTracksCount, &row.DislikedTracksCount, &row.AvgUserRating, &row.CurvedAffinityScore)
	if err != nil {
		t.Fatalf("failed to query clamped affinity row: %v", err)
	}

	if row.FavoriteTracksCount != 2 {
		t.Fatalf("FavoriteTracksCount = %d, want 2", row.FavoriteTracksCount)
	}
	if row.DislikedTracksCount != 2 {
		t.Fatalf("DislikedTracksCount = %d, want 2", row.DislikedTracksCount)
	}
	if !floatClose(row.CurvedAffinityScore, 25.0) {
		t.Fatalf("CurvedAffinityScore = %v, want 25", row.CurvedAffinityScore)
	}
}

func TestFetchTopArtistAffinitiesByGenre(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	db, err := InitDB(filepath.Join(t.TempDir(), "genre-affinity.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	_, err = db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres, user_rating, track_count)
		VALUES
			('album-1', 'Crack the Skye', 'Mastodon', 'crack the skye', 'mastodon', '["progressive metal","sludge metal"]', 4.5, 7),
			('album-2', 'Heaven or Las Vegas', 'Cocteau Twins', 'heaven or las vegas', 'cocteau twins', '["dream pop"]', 5.0, 10),
			('album-3', 'Rated Death Album', 'Rated Death Artist', 'rated death album', 'rated death artist', '["Death Metal"]', 4.0, 9),
			('album-4', 'No Signal Album', 'No Signal Artist', 'no signal album', 'no signal artist', '["death metal"]', NULL, NULL);
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist, genres, is_favorite)
		VALUES
			('track-1', 'Oblivion', 'Crack the Skye', 'Mastodon', 'oblivion', 'mastodon', '["progressive metal"]', 1),
			('track-2', 'Hammer Smashed Face', 'Tomb of the Mutilated', 'Cannibal Corpse', 'hammer smashed face', 'cannibal corpse', '["death metal"]', 1),
			('track-3', 'Stripped, Raped and Strangled', 'The Bleeding', 'Cannibal Corpse', 'stripped raped and strangled', 'cannibal corpse', '["Death Metal"]', 1),
			('track-4', 'Cherry-coloured Funk', 'Heaven or Las Vegas', 'Cocteau Twins', 'cherry coloured funk', 'cocteau twins', '["dream pop"]', 1),
			('track-5', 'No Explicit Signal', 'No Signal Album', 'No Signal Artist', 'no explicit signal', 'no signal artist', '["death metal"]', 0);
	`)
	if err != nil {
		t.Fatalf("failed to insert genre affinity fixtures: %v", err)
	}

	got, err := FetchTopArtistAffinitiesByGenre(context.Background(), db.Ctx, "DeAtH MeTaL", 10)
	if err != nil {
		t.Fatalf("FetchTopArtistAffinitiesByGenre() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2; rows = %#v", len(got), got)
	}
	if got[0].CleanArtist != "rated death artist" {
		t.Fatalf("got[0].CleanArtist = %q, want rated death artist", got[0].CleanArtist)
	}
	if !got[0].AvgUserRating.Valid || !floatClose(got[0].AvgUserRating.Float64, 4.0) {
		t.Fatalf("rated death artist avg rating = %#v, want 4.0", got[0].AvgUserRating)
	}
	if !floatClose(got[0].CurvedAffinityScore, 9.0) {
		t.Fatalf("rated death artist curved affinity score = %v, want 9", got[0].CurvedAffinityScore)
	}
	if got[1].CleanArtist != "cannibal corpse" {
		t.Fatalf("got[1].CleanArtist = %q, want cannibal corpse", got[1].CleanArtist)
	}
	if got[1].FavoriteTracksCount != 2 {
		t.Fatalf("cannibal corpse favorite count = %d, want 2", got[1].FavoriteTracksCount)
	}
	if !floatClose(got[1].CurvedAffinityScore, 2.0) {
		t.Fatalf("cannibal corpse curved affinity score = %v, want 2", got[1].CurvedAffinityScore)
	}

	limited, err := FetchTopArtistAffinitiesByGenre(context.Background(), db.Ctx, "death metal", 1)
	if err != nil {
		t.Fatalf("limited FetchTopArtistAffinitiesByGenre() error = %v", err)
	}
	if len(limited) != 1 || limited[0].CleanArtist != "rated death artist" {
		t.Fatalf("limited FetchTopArtistAffinitiesByGenre() = %#v, want rated death artist only", limited)
	}
}

func TestGenreTopographyViewAggregatesJSONArrays(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	db, err := InitDB(filepath.Join(t.TempDir(), "topography.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	_, err = db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres, user_rating)
		VALUES
			('album-1', 'Crack the Skye', 'Mastodon', 'crack the skye', 'mastodon', '["progressive metal","sludge metal"]', 4.5),
			('album-2', 'Leviathan', 'Mastodon', 'leviathan', 'mastodon', '["progressive metal"]', 5.0),
			('album-3', 'Malformed', 'Artist', 'malformed', 'artist', 'not-json', 4.0),
			('album-4', 'Album Only Genre', 'Artist', 'album only genre', 'artist', '["jazz fusion"]', NULL);
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist, genres, is_favorite)
		VALUES
			('track-1', 'Oblivion', 'Crack the Skye', 'Mastodon', 'oblivion', 'mastodon', '["progressive metal","sludge metal"]', 1),
			('track-2', 'Blood and Thunder', 'Leviathan', 'Mastodon', 'blood and thunder', 'mastodon', '["progressive metal"]', 1),
			('track-3', 'Halo', 'I Am... Sasha Fierce', 'Beyonce', 'halo', 'beyonce', '["pop"]', 0),
			('track-4', 'Malformed', 'Single', 'Artist', 'malformed', 'artist', 'not-json', 1);
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist, genres, is_disliked)
		VALUES ('track-5', 'Suppressed Pop', 'Single', 'Beyonce', 'suppressed pop', 'beyonce', '["pop"]', 1);
	`)
	if err != nil {
		t.Fatalf("failed to insert genre topography fixtures: %v", err)
	}

	var viewSQL string
	err = db.Ctx.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'view' AND name = 'v_genre_topography'").Scan(&viewSQL)
	if err != nil {
		t.Fatalf("failed to inspect v_genre_topography SQL: %v", err)
	}
	if !strings.Contains(viewSQL, "json_each") {
		t.Fatalf("v_genre_topography SQL = %q, want native json_each usage", viewSQL)
	}

	genres, err := FetchTopGenreTopography(context.Background(), db.Ctx, 5)
	if err != nil {
		t.Fatalf("FetchTopGenreTopography() error = %v", err)
	}
	if len(genres) != 4 {
		t.Fatalf("len(genres) = %d, want 4; rows = %#v", len(genres), genres)
	}

	progressive := genres[0]
	if progressive.Subgenre != "progressive metal" {
		t.Fatalf("genres[0].Subgenre = %q, want progressive metal", progressive.Subgenre)
	}
	if progressive.TotalTracks != 2 || progressive.FavoriteTracksCount != 2 {
		t.Fatalf("progressive counts = total %d favorite %d, want 2 and 2", progressive.TotalTracks, progressive.FavoriteTracksCount)
	}
	if progressive.DislikedTracksCount != 0 {
		t.Fatalf("progressive disliked count = %d, want 0", progressive.DislikedTracksCount)
	}
	if !progressive.AvgAlbumRating.Valid || !floatClose(progressive.AvgAlbumRating.Float64, 4.75) {
		t.Fatalf("progressive avg album rating = %#v, want 4.75", progressive.AvgAlbumRating)
	}

	sludge := genres[1]
	if sludge.Subgenre != "sludge metal" || sludge.TotalTracks != 1 || sludge.FavoriteTracksCount != 1 {
		t.Fatalf("genres[1] = %#v, want sludge metal total 1 favorite 1", sludge)
	}
	if !sludge.AvgAlbumRating.Valid || !floatClose(sludge.AvgAlbumRating.Float64, 4.5) {
		t.Fatalf("sludge avg album rating = %#v, want 4.5", sludge.AvgAlbumRating)
	}

	pop := genres[2]
	if pop.Subgenre != "pop" || pop.TotalTracks != 1 || pop.FavoriteTracksCount != 0 || pop.DislikedTracksCount != 1 {
		t.Fatalf("genres[2] = %#v, want pop total 1 favorite 0 disliked 1", pop)
	}
	if pop.AvgAlbumRating.Valid {
		t.Fatalf("pop avg album rating = %#v, want NULL", pop.AvgAlbumRating)
	}

	albumOnly := genres[3]
	if albumOnly.Subgenre != "jazz fusion" || albumOnly.TotalTracks != 0 || albumOnly.FavoriteTracksCount != 0 {
		t.Fatalf("genres[3] = %#v, want jazz fusion total 0 favorite 0", albumOnly)
	}
	if albumOnly.AvgAlbumRating.Valid {
		t.Fatalf("jazz fusion avg album rating = %#v, want NULL", albumOnly.AvgAlbumRating)
	}
}

func floatClose(got, want float64) bool {
	return math.Abs(got-want) < 0.000001
}

func unsetMusicVaultDBPathEnv(t *testing.T) {
	t.Helper()

	value, exists := os.LookupEnv(MusicVaultDBPathEnv)
	if err := os.Unsetenv(MusicVaultDBPathEnv); err != nil {
		t.Fatalf("failed to unset %s: %v", MusicVaultDBPathEnv, err)
	}
	t.Cleanup(func() {
		if exists {
			os.Setenv(MusicVaultDBPathEnv, value)
		} else {
			os.Unsetenv(MusicVaultDBPathEnv)
		}
	})
}
