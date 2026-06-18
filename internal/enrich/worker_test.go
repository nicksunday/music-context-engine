package enrich

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nicksunday/music-context-platform/internal/database"
)

func TestWorkerRunFetchesFiltersAndCommitsGenres(t *testing.T) {
	db := newTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist)
		VALUES ('album-1', 'Destroy Erase Improve', 'Meshuggah', 'destroy erase improve', 'meshuggah');
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist)
		VALUES ('track-1', 'Future Breed Machine', 'Destroy Erase Improve', 'Meshuggah', 'future breed machine', 'meshuggah');
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist, genres)
		VALUES ('track-2', 'Already Done', 'Destroy Erase Improve', 'Meshuggah', 'already done', 'meshuggah', '["existing"]');`)
	if err != nil {
		t.Fatalf("failed to insert test rows: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "test-agent/1.0" {
			t.Errorf("User-Agent = %q, want %q", got, "test-agent/1.0")
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mb/artist":
			if got := r.URL.Query().Get("fmt"); got != "json" {
				t.Errorf("musicbrainz fmt = %q, want json", got)
			}
			w.Write([]byte(`{"artists":[{"id":"mbid-1"}]}`))
		case "/mb/artist/mbid-1":
			w.Write([]byte(`{
				"genres": [
					{"name": "Djent", "count": 100},
					{"name": "Progressive Metal", "count": 80}
				],
				"tags": [
					{"name": "Technical Death Metal", "count": 30}
				]
			}`))
		case "/mb/release":
			if got := r.URL.Query().Get("fmt"); got != "json" {
				t.Errorf("musicbrainz release fmt = %q, want json", got)
			}
			w.Write([]byte(`{"releases":[{"id":"release-1"}]}`))
		case "/mb/release/release-1":
			w.Write([]byte(`{
				"genres": [
					{"name": "Djent", "count": 100},
					{"name": "Progressive Metal", "count": 80}
				],
				"tags": [
					{"name": "Technical Death Metal", "count": 30},
					{"name": "metalcore", "count": 20},
					{"name": "Experimental", "count": 10}
				],
				"media": [
					{"track-count": 8}
				]
			}`))
		case "/lastfm":
			if got := r.URL.Query().Get("api_key"); got != "test-key" {
				t.Errorf("api_key = %q, want test-key", got)
			}
			w.Write([]byte(`{
				"toptags": {
					"tag": [
						{"name": "seen live", "count": "9999"},
						{"name": "Technical Death Metal", "count": "200"},
						{"name": "metalcore", "count": "150"},
						{"name": "awesome", "count": "100"},
						{"name": "Experimental", "count": "5"}
					]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	worker := NewWorker(db.Ctx, Config{
		MusicBrainzBaseURL: server.URL + "/mb",
		LastFMBaseURL:      server.URL + "/lastfm",
		LastFMAPIKey:       "test-key",
		UserAgent:          "test-agent/1.0",
		DisableRateLimit:   true,
		MaxGenres:          10,
	})

	result, err := worker.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ArtistsScanned != 1 {
		t.Fatalf("ArtistsScanned = %d, want 1", result.ArtistsScanned)
	}
	if result.AlbumsScanned != 1 {
		t.Fatalf("AlbumsScanned = %d, want 1", result.AlbumsScanned)
	}
	if result.AlbumsUpdated != 1 {
		t.Fatalf("AlbumsUpdated = %d, want 1", result.AlbumsUpdated)
	}
	if result.ArtistsUpdated != 1 {
		t.Fatalf("ArtistsUpdated = %d, want 1", result.ArtistsUpdated)
	}
	if result.RecordsUpdated != 2 {
		t.Fatalf("RecordsUpdated = %d, want 2", result.RecordsUpdated)
	}

	want := []string{"djent", "progressive metal", "technical death metal", "metalcore", "experimental"}
	assertGenres(t, db.Ctx.QueryRow("SELECT genres FROM albums WHERE id = 'album-1'"), want)
	assertGenres(t, db.Ctx.QueryRow("SELECT genres FROM tracks WHERE id = 'track-1'"), want)
	assertGenres(t, db.Ctx.QueryRow("SELECT genres FROM tracks WHERE id = 'track-2'"), []string{"existing"})
	assertTrackCount(t, db.Ctx.QueryRow("SELECT track_count FROM albums WHERE id = 'album-1'"), 8)
}

func TestWorkerRunMarksMusicBrainzZeroResultAsProcessedAndContinues(t *testing.T) {
	db := newTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist)
		VALUES
			('track-missing', 'Loose Metadata', '', '5th Element/Shock-G/Clev MC/Delina Dream/Ant Dog', 'loose metadata', '5th element shock g clev mc delina dream ant dog'),
			('track-found', 'Future Breed Machine', 'Destroy Erase Improve', 'Meshuggah', 'future breed machine', 'meshuggah');`)
	if err != nil {
		t.Fatalf("failed to insert test rows: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/artist":
			if strings.Contains(r.URL.Query().Get("query"), "5th Element") {
				w.Write([]byte(`{"artists":[]}`))
				return
			}
			w.Write([]byte(`{"artists":[{"id":"mbid-found"}]}`))
		case "/artist/mbid-found":
			w.Write([]byte(`{"genres":[{"name":"Metal","count":100}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var logs bytes.Buffer
	worker := NewWorker(db.Ctx, Config{
		MusicBrainzBaseURL: server.URL,
		DisableRateLimit:   true,
		Logger:             log.New(&logs, "", 0),
	})

	result, err := worker.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ArtistsScanned != 2 {
		t.Fatalf("ArtistsScanned = %d, want 2", result.ArtistsScanned)
	}
	if result.ArtistsUpdated != 1 {
		t.Fatalf("ArtistsUpdated = %d, want 1", result.ArtistsUpdated)
	}
	if result.RecordsUpdated != 2 {
		t.Fatalf("RecordsUpdated = %d, want 2", result.RecordsUpdated)
	}

	assertGenres(t, db.Ctx.QueryRow("SELECT genres FROM tracks WHERE id = 'track-missing'"), []string{})
	assertGenres(t, db.Ctx.QueryRow("SELECT genres FROM tracks WHERE id = 'track-found'"), []string{"metal"})

	wantLog := "Artist 5th Element/Shock-G/Clev MC/Delina Dream/Ant Dog not found externally. Skipping."
	if !strings.Contains(logs.String(), wantLog) {
		t.Fatalf("logs = %q, want to contain %q", logs.String(), wantLog)
	}
}

func TestWorkerRunMarksLastFMCodeSixAsProcessed(t *testing.T) {
	db := newTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist)
		VALUES ('track-1', 'Loose Metadata', '', 'Missing LastFM Artist', 'loose metadata', 'missing lastfm artist');`)
	if err != nil {
		t.Fatalf("failed to insert test row: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/mb/artist":
			w.Write([]byte(`{"artists":[{"id":"mbid-empty"}]}`))
		case "/mb/artist/mbid-empty":
			w.Write([]byte(`{"genres":[],"tags":[]}`))
		case "/lastfm":
			w.Write([]byte(`{"error":6,"message":"The artist you supplied could not be found"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var logs bytes.Buffer
	worker := NewWorker(db.Ctx, Config{
		MusicBrainzBaseURL: server.URL + "/mb",
		LastFMBaseURL:      server.URL + "/lastfm",
		LastFMAPIKey:       "test-key",
		DisableRateLimit:   true,
		Logger:             log.New(&logs, "", 0),
	})

	result, err := worker.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.RecordsUpdated != 1 {
		t.Fatalf("RecordsUpdated = %d, want 1", result.RecordsUpdated)
	}

	assertGenres(t, db.Ctx.QueryRow("SELECT genres FROM tracks WHERE id = 'track-1'"), []string{})

	wantLog := "Artist Missing LastFM Artist not found externally. Skipping."
	if !strings.Contains(logs.String(), wantLog) {
		t.Fatalf("logs = %q, want to contain %q", logs.String(), wantLog)
	}
}

func TestWorkerRateLimitsOutboundRequests(t *testing.T) {
	var requestTimes []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestTimes = append(requestTimes, time.Now())
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/artist":
			w.Write([]byte(`{"artists":[{"id":"mbid-1"}]}`))
		case "/artist/mbid-1":
			w.Write([]byte(`{"genres":[{"name":"metal","count":1}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	worker := NewWorker(nil, Config{
		MusicBrainzBaseURL: server.URL,
		RequestDelay:       25 * time.Millisecond,
		LastFMAPIKey:       "",
	})

	_, err := worker.FetchArtistGenres(context.Background(), "Test Artist")
	if err != nil {
		t.Fatalf("FetchArtistGenres() error = %v", err)
	}

	if len(requestTimes) != 2 {
		t.Fatalf("len(requestTimes) = %d, want 2", len(requestTimes))
	}
	if elapsed := requestTimes[1].Sub(requestTimes[0]); elapsed < 20*time.Millisecond {
		t.Fatalf("elapsed between requests = %s, want at least 20ms", elapsed)
	}
}

func newTestDB(t *testing.T) *database.DBClient {
	t.Helper()

	unsetMusicVaultDBPathEnv(t)

	db, err := database.InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		db.Ctx.Close()
	})

	return db
}

func unsetMusicVaultDBPathEnv(t *testing.T) {
	t.Helper()

	value, exists := os.LookupEnv(database.MusicVaultDBPathEnv)
	if err := os.Unsetenv(database.MusicVaultDBPathEnv); err != nil {
		t.Fatalf("failed to unset %s: %v", database.MusicVaultDBPathEnv, err)
	}
	t.Cleanup(func() {
		if exists {
			os.Setenv(database.MusicVaultDBPathEnv, value)
		} else {
			os.Unsetenv(database.MusicVaultDBPathEnv)
		}
	})
}

type genreRow interface {
	Scan(dest ...any) error
}

func assertGenres(t *testing.T, row genreRow, want []string) {
	t.Helper()

	var raw string
	if err := row.Scan(&raw); err != nil {
		t.Fatalf("failed to scan genres: %v", err)
	}

	var got []string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("genres %q is not valid JSON: %v", raw, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("genres = %#v, want %#v", got, want)
	}
}

func assertTrackCount(t *testing.T, row genreRow, want int64) {
	t.Helper()

	var got int64
	if err := row.Scan(&got); err != nil {
		t.Fatalf("failed to scan track_count: %v", err)
	}
	if got != want {
		t.Fatalf("track_count = %d, want %d", got, want)
	}
}
