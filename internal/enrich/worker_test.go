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

func TestWorkerRunSkipsAlbumAfterExhaustingRetriesOnTransientError(t *testing.T) {
	db := newTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist)
		VALUES ('album-1', 'Dare to Be Stupid', '"Weird Al" Yankovic', 'dare to be stupid', 'weird al yankovic');`)
	if err != nil {
		t.Fatalf("failed to insert test row: %v", err)
	}

	var releaseSearchRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/mb/release":
			releaseSearchRequests++
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error": "The MusicBrainz web server is currently busy. Please try again later."}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var logs bytes.Buffer
	worker := NewWorker(db.Ctx, Config{
		MusicBrainzBaseURL:  server.URL + "/mb",
		DisableRateLimit:    true,
		MaxTransientRetries: 2,
		TransientRetryDelay: time.Millisecond,
		Logger:              log.New(&logs, "", 0),
	})

	result, err := worker.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v, want the transient failure to be skipped rather than aborting the run", err)
	}
	if result.AlbumsSkippedTransient != 1 {
		t.Fatalf("AlbumsSkippedTransient = %d, want 1", result.AlbumsSkippedTransient)
	}
	if result.AlbumsUpdated != 0 {
		t.Fatalf("AlbumsUpdated = %d, want 0 (skipped album should not be committed)", result.AlbumsUpdated)
	}

	// 1 initial attempt + 2 retries = 3 total requests to the release search endpoint.
	if releaseSearchRequests != 3 {
		t.Fatalf("releaseSearchRequests = %d, want 3 (initial attempt + MaxTransientRetries retries)", releaseSearchRequests)
	}

	wantLog := "Skipping album Dare to Be Stupid"
	if !strings.Contains(logs.String(), wantLog) {
		t.Fatalf("logs = %q, want to contain %q", logs.String(), wantLog)
	}
	wantRetryLog := "Retrying"
	if !strings.Contains(logs.String(), wantRetryLog) {
		t.Fatalf("logs = %q, want to contain a %q log line", logs.String(), wantRetryLog)
	}
}

func TestWorkerRunBackfillsEmptyGenreArraysFromLinkedRows(t *testing.T) {
	db := newTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres, track_count)
		VALUES
			('album-from-track', 'Tagged By Tracks', 'Local Artist', 'tagged by tracks', 'local artist', '[]', 8),
			('album-to-track', 'Tagged Album', 'Album Artist', 'tagged album', 'album artist', '["progressive rock","jazz fusion"]', 9);
		INSERT INTO tracks (id, album_id, title, album, artist, clean_title, clean_artist, genres)
		VALUES
			('track-source-1', 'album-from-track', 'Source One', 'Tagged By Tracks', 'Local Artist', 'source one', 'local artist', '["jazz fusion","progressive rock"]'),
			('track-source-2', 'album-from-track', 'Source Two', 'Tagged By Tracks', 'Local Artist', 'source two', 'local artist', '["jazz fusion"]'),
			('track-target', 'album-to-track', 'Needs Tags', 'Tagged Album', 'Album Artist', 'needs tags', 'album artist', '[]');`)
	if err != nil {
		t.Fatalf("failed to insert test rows: %v", err)
	}

	worker := NewWorker(db.Ctx, Config{DisableRateLimit: true})
	result, err := worker.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.RecordsUpdated != 2 {
		t.Fatalf("RecordsUpdated = %d, want 2", result.RecordsUpdated)
	}

	assertGenres(t, db.Ctx.QueryRow("SELECT genres FROM albums WHERE id = 'album-from-track'"), []string{"jazz fusion", "progressive rock"})
	assertGenres(t, db.Ctx.QueryRow("SELECT genres FROM tracks WHERE id = 'track-target'"), []string{"progressive rock", "jazz fusion"})
}

func TestWorkerRunUsesDecodedAlbumLookupForEmptyGenres(t *testing.T) {
	db := newTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres)
		VALUES ('album-1', 'Brain Salad Surgery', 'Emerson, Lake &amp; Palmer', 'brain salad surgery', 'emerson lake and amp palmer', '[]');`)
	if err != nil {
		t.Fatalf("failed to insert test row: %v", err)
	}

	var sawDecodedArtistQuery bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/mb/release":
			query := r.URL.Query().Get("query")
			if strings.Contains(query, `artist:"emerson lake and palmer"`) {
				sawDecodedArtistQuery = true
				w.Write([]byte(`{"releases":[{"id":"release-1"}]}`))
				return
			}
			w.Write([]byte(`{"releases":[]}`))
		case "/mb/release/release-1":
			w.Write([]byte(`{
				"genres": [
					{"name": "Progressive Rock", "count": 100},
					{"name": "Symphonic Prog", "count": 60}
				],
				"media": [{"track-count": 8}]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	worker := NewWorker(db.Ctx, Config{
		MusicBrainzBaseURL: server.URL + "/mb",
		DisableRateLimit:   true,
	})

	result, err := worker.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.AlbumsUpdated != 1 {
		t.Fatalf("AlbumsUpdated = %d, want 1", result.AlbumsUpdated)
	}
	if !sawDecodedArtistQuery {
		t.Fatalf("MusicBrainz release search did not retry with decoded artist text")
	}

	assertGenres(t, db.Ctx.QueryRow("SELECT genres FROM albums WHERE id = 'album-1'"), []string{"progressive rock", "symphonic prog"})
	assertTrackCount(t, db.Ctx.QueryRow("SELECT track_count FROM albums WHERE id = 'album-1'"), 8)
}

func TestWorkerRunUsesReleaseGroupFallbackForEmptyGenres(t *testing.T) {
	db := newTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres, track_count)
		VALUES ('album-1', 'The Dethalbum (Expanded Edition)', 'Metalocalypse: Dethklok', 'the dethalbum expanded edition', 'metalocalypse dethklok', '[]', 24);`)
	if err != nil {
		t.Fatalf("failed to insert test row: %v", err)
	}

	var sawStrippedReleaseGroupQuery bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/mb/release":
			w.Write([]byte(`{"releases":[]}`))
		case "/mb/release-group":
			query := r.URL.Query().Get("query")
			if strings.Contains(query, `releasegroup:"the dethalbum"`) {
				sawStrippedReleaseGroupQuery = true
				w.Write([]byte(`{"release-groups":[{"id":"rg-1"}]}`))
				return
			}
			w.Write([]byte(`{"release-groups":[]}`))
		case "/mb/release-group/rg-1":
			w.Write([]byte(`{
				"tags": [
					{"name": "Melodic Death Metal", "count": 70},
					{"name": "Comedy Metal", "count": 20}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	worker := NewWorker(db.Ctx, Config{
		MusicBrainzBaseURL: server.URL + "/mb",
		DisableRateLimit:   true,
	})

	result, err := worker.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.AlbumsUpdated != 1 {
		t.Fatalf("AlbumsUpdated = %d, want 1", result.AlbumsUpdated)
	}
	if !sawStrippedReleaseGroupQuery {
		t.Fatalf("MusicBrainz release-group search did not use stripped title fallback")
	}

	assertGenres(t, db.Ctx.QueryRow("SELECT genres FROM albums WHERE id = 'album-1'"), []string{"melodic death metal", "comedy metal"})
}

func TestWorkerRunUsesLastFMAlbumTagsWhenMusicBrainzAlbumTagsAreEmpty(t *testing.T) {
	db := newTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres, track_count)
		VALUES ('album-1', 'Infest the Rats'' Nest', 'King Gizzard & The Lizard Wizard', 'infest the rats nest', 'king gizzard and the lizard wizard', '[]', 9);`)
	if err != nil {
		t.Fatalf("failed to insert test row: %v", err)
	}

	var sawLastFMAlbumTags bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/mb/release":
			w.Write([]byte(`{"releases":[{"id":"release-1"}]}`))
		case "/mb/release/release-1":
			w.Write([]byte(`{"genres":[],"tags":[],"media":[{"track-count":9}]}`))
		case "/mb/release-group":
			w.Write([]byte(`{"release-groups":[]}`))
		case "/lastfm":
			if got := r.URL.Query().Get("method"); got == "album.gettoptags" {
				sawLastFMAlbumTags = true
				w.Write([]byte(`{
					"toptags": {
						"tag": [
							{"name": "seen live", "count": "999"},
							{"name": "Thrash Metal", "count": "100"},
							{"name": "Psychedelic Rock", "count": "80"},
							{"name": "Jazz Rap", "count": "70"},
							{"name": "1990s", "count": "60"}
						]
					}
				}`))
				return
			}
			w.Write([]byte(`{"toptags":{"tag":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	worker := NewWorker(db.Ctx, Config{
		MusicBrainzBaseURL: server.URL + "/mb",
		LastFMBaseURL:      server.URL + "/lastfm",
		LastFMAPIKey:       "test-key",
		DisableRateLimit:   true,
	})

	result, err := worker.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.AlbumsUpdated != 1 {
		t.Fatalf("AlbumsUpdated = %d, want 1", result.AlbumsUpdated)
	}
	if !sawLastFMAlbumTags {
		t.Fatalf("Last.fm album.gettoptags fallback was not called")
	}

	assertGenres(t, db.Ctx.QueryRow("SELECT genres FROM albums WHERE id = 'album-1'"), []string{"thrash metal", "psychedelic rock", "jazz rap"})
}

func TestUsefulLastFMTagRejectsNoiseWithoutGenreWhitelist(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "seen live", want: false},
		{name: "Awesome", want: false},
		{name: "albums I own", want: false},
		{name: "1990s", want: false},
		{name: "jazz rap", want: true},
		{name: "new weird america", want: true},
		{name: "zeuhl", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUsefulLastFMTag(tt.name); got != tt.want {
				t.Fatalf("isUsefulLastFMTag(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
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
