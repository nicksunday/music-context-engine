package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/nicksunday/music-context-platform/internal/utils"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestGetVerifiedDiscoveryCandidatesFetchesMusicBrainzAndFiltersExclusions(t *testing.T) {
	sourceServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/recording" {
			t.Errorf("request path = %q, want %q", request.URL.Path, "/recording")
		}
		if got := request.URL.Query().Get("fmt"); got != "json" {
			t.Errorf("fmt = %q, want json", got)
		}
		if got := request.URL.Query().Get("inc"); got != "artist-rels+release-groups+aliases" {
			t.Errorf("inc = %q, want artist-rels+release-groups+aliases", got)
		}
		if got := request.URL.Query().Get("limit"); got != "20" {
			t.Errorf("limit = %q, want 20", got)
		}
		if got := request.URL.Query().Get("query"); got != `(tag:"math rock" OR tag:"idm" OR tag:"breakcore") AND primarytype:album AND status:official` {
			t.Errorf("query = %q", got)
		}
		if got := request.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		if got := request.Header.Get("User-Agent"); got != "discovery-test/1.0" {
			t.Errorf("User-Agent = %q, want discovery-test/1.0", got)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"recordings": [
				{
					"title": "Gantz Graf",
					"length": 238000,
					"first-release-date": "2002-08-05",
					"artist-credit": [{"name": "Autechre"}],
					"releases": [{
						"title": "Gantz Graf",
						"status": "Official",
						"date": "2002-08-05",
						"release-group": {"primary-type": "Album"}
					}]
				},
				{
					"title": "T69 Collapse",
					"length": 322000,
					"first-release-date": "2018-08-07",
					"artist-credit": [{"name": "Aphex Twin"}],
					"releases": [{
						"title": "Collapse EP",
						"status": "Official",
						"date": "2018-09-14",
						"release-group": {"primary-type": "Album"}
					}]
				},
				{
					"title": "Story 2",
					"length": 131000,
					"first-release-date": "2014-06-10",
					"artist-credit": [{"name": "clipping."}],
					"releases": [{
						"title": "CLPPNG",
						"status": "Official",
						"date": "2014-06-10",
						"release-group": {"primary-type": "Album"}
					}]
				},
				{
					"title": "Fracture",
					"length": 671000,
					"first-release-date": "1974-03-29",
					"artist-credit": [{"name": "King Crimson"}],
					"releases": [{
						"title": "Starless and Bible Black",
						"status": "Official",
						"date": "1974-03-29",
						"release-group": {"primary-type": "Album"}
					}]
				},
				{
					"title": "Missing Runtime",
					"length": 0,
					"first-release-date": "2020",
					"artist-credit": [{"name": "Malformed Result"}],
					"releases": [{
						"title": "Incomplete",
						"status": "Official",
						"date": "2020",
						"release-group": {"primary-type": "Album"}
					}]
				}
			]
		}`))
	}))
	defer sourceServer.Close()

	source := newMusicBrainzDiscoveryClient(musicBrainzDiscoveryConfig{
		HTTPClient:   sourceServer.Client(),
		BaseURL:      sourceServer.URL,
		UserAgent:    "discovery-test/1.0",
		Timeout:      time.Second,
		RequestDelay: 0,
	})
	candidates, err := getVerifiedDiscoveryCandidates(
		context.Background(),
		source,
		[]string{" Math Rock, IDM ", "breakcore", "idm"},
		2,
		map[string]bool{
			"AUTECHRE!!!": true,
			"collapse ep": true,
			"STORY 2":     true,
		},
	)
	if err != nil {
		t.Fatalf("getVerifiedDiscoveryCandidates() error = %v", err)
	}

	want := []DiscoveryCandidate{{
		TrackName:   "Fracture",
		Artist:      "King Crimson",
		Album:       "Starless and Bible Black",
		Runtime:     "11:11",
		ReleaseYear: 1974,
	}}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("getVerifiedDiscoveryCandidates() = %#v, want %#v", candidates, want)
	}
}

func TestGetVerifiedDiscoveryCandidatesCapsResultsPerArtist(t *testing.T) {
	candidates, err := getVerifiedDiscoveryCandidates(
		context.Background(),
		discoverySourceFunc(func(context.Context, []string, int) ([]DiscoveryCandidate, error) {
			return []DiscoveryCandidate{
				{TrackName: "First", Artist: "Mono Band", Album: "First Album", Runtime: "3:01", ReleaseYear: 2020},
				{TrackName: "Second", Artist: "Mono Band", Album: "Second Album", Runtime: "3:02", ReleaseYear: 2021},
				{TrackName: "Third", Artist: "Mono Band", Album: "Third Album", Runtime: "3:03", ReleaseYear: 2022},
				{TrackName: "Other One", Artist: "Other Project", Album: "Other Album", Runtime: "3:04", ReleaseYear: 2023},
				{TrackName: "Another One", Artist: "Another Project", Album: "Another Album", Runtime: "3:05", ReleaseYear: 2024},
			}, nil
		}),
		[]string{"funk metal"},
		5,
		nil,
	)
	if err != nil {
		t.Fatalf("getVerifiedDiscoveryCandidates() error = %v", err)
	}

	want := []DiscoveryCandidate{
		{TrackName: "First", Artist: "Mono Band", Album: "First Album", Runtime: "3:01", ReleaseYear: 2020},
		{TrackName: "Second", Artist: "Mono Band", Album: "Second Album", Runtime: "3:02", ReleaseYear: 2021},
		{TrackName: "Other One", Artist: "Other Project", Album: "Other Album", Runtime: "3:04", ReleaseYear: 2023},
		{TrackName: "Another One", Artist: "Another Project", Album: "Another Album", Runtime: "3:05", ReleaseYear: 2024},
	}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("getVerifiedDiscoveryCandidates() = %#v, want %#v", candidates, want)
	}
}

func TestGetVerifiedDiscoveryCandidatesCapsResultsPerNormalizedArtist(t *testing.T) {
	candidates, err := getVerifiedDiscoveryCandidates(
		context.Background(),
		discoverySourceFunc(func(context.Context, []string, int) ([]DiscoveryCandidate, error) {
			return []DiscoveryCandidate{
				{TrackName: "Svefn-g-englar", Artist: "Sigur Rós", Album: "Ágætis byrjun", Runtime: "10:04", ReleaseYear: 1999},
				{TrackName: "Starálfur", Artist: "Sigur Ros", Album: "Agaetis byrjun", Runtime: "6:47", ReleaseYear: 1999},
				{TrackName: "Glósóli", Artist: "Sigur Rós", Album: "Takk...", Runtime: "6:15", ReleaseYear: 2005},
				{TrackName: "Hyperballad", Artist: "Björk", Album: "Post", Runtime: "5:21", ReleaseYear: 1995},
			}, nil
		}),
		[]string{"post rock"},
		4,
		nil,
	)
	if err != nil {
		t.Fatalf("getVerifiedDiscoveryCandidates() error = %v", err)
	}

	normalizedArtist := "sigur ros"
	normalizedArtistCount := 0
	controlIncluded := false
	for _, candidate := range candidates {
		cleanArtist, err := utils.NormalizeSearchText(candidate.Artist)
		if err != nil {
			t.Fatalf("NormalizeSearchText(%q) error = %v", candidate.Artist, err)
		}
		if cleanArtist == normalizedArtist {
			normalizedArtistCount++
		}
		if candidate.Artist == "Björk" && candidate.TrackName == "Hyperballad" {
			controlIncluded = true
		}
	}

	if normalizedArtistCount > 2 {
		t.Fatalf("normalized artist count = %d, want at most 2; candidates = %#v", normalizedArtistCount, candidates)
	}
	if !controlIncluded {
		t.Fatalf("getVerifiedDiscoveryCandidates() = %#v, want control candidate included", candidates)
	}
}

func TestDiscoverySearchLimitPadsAndCapsExternalFetches(t *testing.T) {
	tests := []struct {
		limit int
		want  int
	}{
		{limit: 1, want: 20},
		{limit: 5, want: 20},
		{limit: 6, want: 24},
		{limit: 25, want: 100},
		{limit: 50, want: 100},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("limit_%d", tt.limit), func(t *testing.T) {
			if got := discoverySearchLimit(tt.limit); got != tt.want {
				t.Fatalf("discoverySearchLimit(%d) = %d, want %d", tt.limit, got, tt.want)
			}
		})
	}
}

func TestParseMusicBrainzCandidatesAppendsRomanizedAliases(t *testing.T) {
	var response musicBrainzRecordingSearchResponse
	if err := json.Unmarshal([]byte(`{
		"recordings": [
			{
				"title": "斑",
				"length": 216226,
				"first-release-date": "2016-07-13",
				"aliases": [
					{"name": "Madra", "locale": "en", "type": "Search hint", "primary": true},
					{"name": "Madara", "locale": "en", "type": "Recording name", "primary": true}
				],
				"artist-credit": [{"name": "Develop One's Faculties"}],
				"releases": [{
					"title": "不恰好な街と僕と君",
					"status": "Official",
					"date": "2016-07-13",
					"release-group": {
						"id": "release-group-1",
						"title": "不恰好な街と僕と君",
						"primary-type": "Album",
						"aliases": [{
							"name": "Bukakkou na Machi to Boku to Kimi",
							"locale": "en",
							"type": "Release group name",
							"primary": true
						}]
					}
				}]
			},
			{
				"title": "悲しいKiss",
				"length": 351666,
				"first-release-date": "1989-03-21",
				"artist-credit": [{"name": "DREAMS COME TRUE"}],
				"releases": [
					{
						"title": "非幸福論",
						"status": "Official",
						"date": "1989-03-21",
						"release-group": {
							"id": "release-group-2",
							"title": "非幸福論",
							"primary-type": "Album"
						}
					},
					{
						"title": "Hikoufukuron",
						"status": "Pseudo-Release",
						"release-group": {
							"id": "release-group-2",
							"title": "非幸福論",
							"primary-type": "Album"
						},
						"media": [{
							"track": [{"title": "Kanashii Kiss"}]
						}]
					}
				]
			}
		]
	}`), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	candidates := parseMusicBrainzCandidates(response.Recordings)
	if len(candidates) != 2 {
		t.Fatalf("len(parseMusicBrainzCandidates()) = %d, want 2", len(candidates))
	}

	if got, want := candidates[0].TrackName, "斑 (Madara)"; got != want {
		t.Errorf("first TrackName = %q, want %q", got, want)
	}
	if got, want := candidates[0].Album, "不恰好な街と僕と君 (Bukakkou na Machi to Boku to Kimi)"; got != want {
		t.Errorf("first Album = %q, want %q", got, want)
	}
	if got, want := candidates[0].trackExclusionNames, []string{"斑", "Madara"}; !reflect.DeepEqual(got, want) {
		t.Errorf("first trackExclusionNames = %#v, want %#v", got, want)
	}
	if got, want := candidates[0].albumExclusionNames, []string{"不恰好な街と僕と君", "Bukakkou na Machi to Boku to Kimi"}; !reflect.DeepEqual(got, want) {
		t.Errorf("first albumExclusionNames = %#v, want %#v", got, want)
	}
	if got, want := candidates[1].TrackName, "悲しいKiss (Kanashii Kiss)"; got != want {
		t.Errorf("second TrackName = %q, want %q", got, want)
	}
	if got, want := candidates[1].Album, "非幸福論 (Hikoufukuron)"; got != want {
		t.Errorf("second Album = %q, want %q", got, want)
	}
}

func TestGetVerifiedDiscoveryCandidatesFiltersRomanizedAliases(t *testing.T) {
	candidate := DiscoveryCandidate{
		TrackName:           "斑 (Madara)",
		Artist:              "Develop One's Faculties",
		Album:               "不恰好な街と僕と君 (Bukakkou na Machi to Boku to Kimi)",
		Runtime:             "3:36",
		ReleaseYear:         2016,
		trackExclusionNames: []string{"斑", "Madara"},
		albumExclusionNames: []string{"不恰好な街と僕と君", "Bukakkou na Machi to Boku to Kimi"},
	}

	for _, exclusion := range []string{
		"斑",
		"Madara",
		"不恰好な街と僕と君",
		"Bukakkou na Machi to Boku to Kimi",
	} {
		t.Run(exclusion, func(t *testing.T) {
			candidates, err := getVerifiedDiscoveryCandidates(
				context.Background(),
				discoverySourceFunc(func(context.Context, []string, int) ([]DiscoveryCandidate, error) {
					return []DiscoveryCandidate{candidate}, nil
				}),
				[]string{"math rock"},
				1,
				map[string]bool{exclusion: true},
			)
			if err != nil {
				t.Fatalf("getVerifiedDiscoveryCandidates() error = %v", err)
			}
			if len(candidates) != 0 {
				t.Fatalf("getVerifiedDiscoveryCandidates() = %#v, want title variant to be excluded", candidates)
			}
		})
	}
}

func TestMusicBrainzDiscoveryClientAppliesTimeoutContext(t *testing.T) {
	deadlineRemaining := make(chan time.Duration, 1)
	httpClient := &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			deadline, ok := request.Context().Deadline()
			if !ok {
				return nil, fmt.Errorf("request context has no deadline")
			}
			deadlineRemaining <- time.Until(deadline)
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
	}
	source := newMusicBrainzDiscoveryClient(musicBrainzDiscoveryConfig{
		HTTPClient:   httpClient,
		BaseURL:      "https://musicbrainz.test/ws/2",
		Timeout:      50 * time.Millisecond,
		RequestDelay: 0,
	})

	started := time.Now()
	_, err := source.Search(context.Background(), []string{"math rock"}, 5)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Search() error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Search() elapsed = %s, want bounded timeout", elapsed)
	}

	remaining := <-deadlineRemaining
	if remaining <= 0 || remaining > 100*time.Millisecond {
		t.Fatalf("request deadline remaining = %s, want a clear 50ms timeout", remaining)
	}
}
