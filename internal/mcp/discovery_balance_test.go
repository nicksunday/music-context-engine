package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nicksunday/music-context-platform/internal/database"
)

func balanceRecords(artist string, count int) []musicBrainzRecording {
	var out []musicBrainzRecording
	for i := 0; i < count; i++ {
		title := fmt.Sprintf("track-%d", i)
		out = append(out, musicBrainzRecording{
			Title: title, Length: 180000, FirstReleaseDate: "2020-01-01",
			ArtistCredit: []musicBrainzArtistCredit{{Name: artist}},
			Tags:         []musicBrainzTag{{Name: "idm", Count: 1}},
			Releases: []musicBrainzRelease{{
				Title: fmt.Sprintf("album-%d", i/2), Status: "Official", Date: "2020-01-01",
				Group: musicBrainzReleaseGroup{ID: fmt.Sprintf("%s-%d", artist, i/2), PrimaryType: "Album"},
			}},
		})
	}
	return out
}

func TestMergeDiscoveryRecordingsBalance(t *testing.T) {
	a, b, c, d, tags := balanceRecords("A", 60), balanceRecords("B", 60), balanceRecords("C", 60), balanceRecords("D", 60), balanceRecords("new artist", 60)
	for _, tt := range []struct {
		name    string
		tags    []musicBrainzRecording
		artists [][]musicBrainzRecording
		want    map[string]int
	}{
		{"full", tags, [][]musicBrainzRecording{a, b, c, d}, map[string]int{"new artist": 24, "A": 6, "B": 6, "C": 6, "D": 6}},
		{"no tags", nil, [][]musicBrainzRecording{a, b, c, d}, map[string]int{"A": 12, "B": 12, "C": 12, "D": 12}},
		{"sparse tags", tags[:2], [][]musicBrainzRecording{a, b}, map[string]int{"new artist": 2, "A": 23, "B": 23}},
		{"sparse artists", tags, [][]musicBrainzRecording{a[:1]}, map[string]int{"new artist": 47, "A": 1}},
		{"overlap", tags, [][]musicBrainzRecording{append(append([]musicBrainzRecording{}, tags...), a...), b}, map[string]int{"new artist": 36, "B": 12}},
		{"empty", nil, nil, map[string]int{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeDiscoveryRecordings(tt.tags, tt.artists)
			counts := map[string]int{}
			seen := map[string]bool{}
			for _, r := range got {
				key := musicBrainzRecordingKey(r)
				if seen[key] {
					t.Fatal("duplicate", key)
				}
				seen[key] = true
				counts[musicBrainzArtistName(r.ArtistCredit)]++
			}
			if !reflect.DeepEqual(counts, tt.want) {
				t.Fatalf("counts %v, want %v", counts, tt.want)
			}
			if len(got) > 48 {
				t.Fatal("pool cap exceeded")
			}
		})
	}
}

func TestSeededDiscoveryRequestsAndFailures(t *testing.T) {
	for _, failure := range []string{"", "tags", "A"} {
		t.Run("failure="+failure, func(t *testing.T) {
			var calls []string
			var times []time.Time
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				source := "tags"
				for _, name := range []string{"A", "B", "C", "D"} {
					if strings.Contains(r.URL.Query().Get("query"), `artist:"`+name+`"`) {
						source = name
					}
				}
				calls = append(calls, source)
				times = append(times, time.Now())
				if r.URL.Query().Get("limit") != "60" || r.URL.Query().Get("offset") != "" {
					t.Error("unexpected request budget", r.URL)
				}
				if r.Header.Get("User-Agent") != "balance-test" {
					t.Error("missing client identification")
				}
				if source == failure {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				json.NewEncoder(w).Encode(musicBrainzRecordingSearchResponse{Recordings: balanceRecords(source, 60)})
			}))
			defer server.Close()
			client := newMusicBrainzDiscoveryClient(musicBrainzDiscoveryConfig{HTTPClient: server.Client(), BaseURL: server.URL, UserAgent: "balance-test", Timeout: time.Second, RequestDelay: 5 * time.Millisecond})
			got, err := client.searchSeededRecordings(context.Background(), []string{"idm"}, []string{"A", " a ", "B", "C", "D", "E"}, 60)
			if err != nil || len(got) != 48 {
				t.Fatalf("records=%d err=%v", len(got), err)
			}
			if !reflect.DeepEqual(calls, []string{"tags", "A", "B", "C", "D"}) {
				t.Fatalf("calls %v", calls)
			}
			for i := 1; i < len(times); i++ {
				if times[i].Sub(times[i-1]) < 5*time.Millisecond {
					t.Fatal("requests were not rate limited")
				}
			}
		})
	}
}

func TestSeededDiscoveryCancellationStopsSources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		cancel()
		json.NewEncoder(w).Encode(musicBrainzRecordingSearchResponse{})
	}))
	defer server.Close()
	client := newMusicBrainzDiscoveryClient(musicBrainzDiscoveryConfig{HTTPClient: server.Client(), BaseURL: server.URL, Timeout: time.Second})
	if _, err := client.searchSeededRecordings(ctx, []string{"idm"}, []string{"A", "B"}, 60); err == nil {
		t.Fatal("expected cancellation")
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestBalancedDiscoveryModesPreserveEligibility(t *testing.T) {
	for _, song := range []bool{false, true} {
		t.Run(fmt.Sprintf("song=%v", song), func(t *testing.T) {
			lookups := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/release-group/") {
					lookups++
					fmt.Fprint(w, `{"genres":[{"name":"idm","count":1}]}`)
					return
				}
				artist := "unfamiliar"
				if strings.Contains(r.URL.Query().Get("query"), `artist:`) {
					artist = "seed"
				}
				json.NewEncoder(w).Encode(musicBrainzRecordingSearchResponse{Recordings: balanceRecords(artist, 60)})
			}))
			defer server.Close()
			client := newMusicBrainzDiscoveryClient(musicBrainzDiscoveryConfig{HTTPClient: server.Client(), BaseURL: server.URL, Timeout: time.Second})
			exclusions := database.NewAlbumExclusionSet()
			if err := exclusions.Add("unfamiliar", "album-0"); err != nil {
				t.Fatal(err)
			}
			got, err := getVerifiedDiscoveryCandidatesSeededWithOptions(context.Background(), client, []string{"idm"}, []string{"seed"}, 20, exclusions, song)
			if err != nil {
				t.Fatal(err)
			}
			counts := map[string]int{}
			albums := map[string]bool{}
			for _, c := range got {
				if c.Artist == "unfamiliar" && c.Album == "album-0" {
					t.Fatal("excluded album survived")
				}
				key := c.Artist + "/" + c.Album
				if albums[key] {
					t.Fatal("duplicate album")
				}
				albums[key] = true
				counts[c.Artist]++
			}
			if counts["unfamiliar"] != 2 || counts["seed"] != 2 || len(got) != 4 {
				t.Fatalf("filtered counts=%v", counts)
			}
			if song && lookups != 0 || !song && lookups == 0 {
				t.Fatalf("genre lookups=%d", lookups)
			}
		})
	}
}
