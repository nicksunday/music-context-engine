package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nicksunday/music-context-platform/internal/database"
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
		if got := request.URL.Query().Get("inc"); got != "release-groups+aliases+genres+tags" {
			t.Errorf("inc = %q, want release-groups+aliases+genres+tags", got)
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
					}],
					"tags": [
						{"name": "progressive rock", "count": 12},
						{"name": "canterbury scene", "count": 3},
						{"name": "Progressive Rock", "count": 1}
					]
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
	exclusions := database.NewAlbumExclusionSet()
	for _, album := range []struct {
		artist string
		title  string
	}{
		{artist: "AUTECHRE!!!", title: "Gantz Graf"},
		{artist: "Aphex Twin", title: "collapse ep"},
		{artist: "clipping.", title: "CLPPNG"},
	} {
		if err := exclusions.Add(album.artist, album.title); err != nil {
			t.Fatalf("failed to add exclusion: %v", err)
		}
	}
	candidates, err := getVerifiedDiscoveryCandidates(
		context.Background(),
		source,
		[]string{" Math Rock, IDM ", "breakcore", "idm"},
		2,
		exclusions,
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
		GenreTags:   []string{"progressive rock", "canterbury scene"},
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
		database.NewAlbumExclusionSet(),
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

func TestGetVerifiedDiscoveryCandidatesDeduplicatesAlbums(t *testing.T) {
	candidates, err := getVerifiedDiscoveryCandidates(
		context.Background(),
		discoverySourceFunc(func(context.Context, []string, int) ([]DiscoveryCandidate, error) {
			return []DiscoveryCandidate{
				{TrackName: "Starter One", Artist: "Album Project", Album: "One Album", Runtime: "3:01", ReleaseYear: 2020},
				{TrackName: "Starter Two", Artist: "Album Project", Album: "One Album", Runtime: "3:02", ReleaseYear: 2020},
				{TrackName: "Next Album", Artist: "Album Project", Album: "Second Album", Runtime: "3:03", ReleaseYear: 2021},
				{TrackName: "Other One", Artist: "Other Project", Album: "Other Album", Runtime: "3:04", ReleaseYear: 2022},
			}, nil
		}),
		[]string{"industrial"},
		4,
		database.NewAlbumExclusionSet(),
	)
	if err != nil {
		t.Fatalf("getVerifiedDiscoveryCandidates() error = %v", err)
	}

	want := []DiscoveryCandidate{
		{TrackName: "Starter One", Artist: "Album Project", Album: "One Album", Runtime: "3:01", ReleaseYear: 2020},
		{TrackName: "Next Album", Artist: "Album Project", Album: "Second Album", Runtime: "3:03", ReleaseYear: 2021},
		{TrackName: "Other One", Artist: "Other Project", Album: "Other Album", Runtime: "3:04", ReleaseYear: 2022},
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
		database.NewAlbumExclusionSet(),
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

func TestDiversifyDiscoveryCandidatesPreservesFitAndVariesTies(t *testing.T) {
	candidates := []DiscoveryCandidate{
		{Artist: "Weak", Album: "Weak Album", TrackName: "Weak Track", GenreTags: []string{"heavy metal"}},
		{Artist: "Tie One", Album: "One", TrackName: "One Track", GenreTags: []string{"progressive metal"}},
		{Artist: "Tie Two", Album: "Two", TrackName: "Two Track", GenreTags: []string{"progressive metal"}},
	}

	first := diversifyDiscoveryCandidatesWithRand(candidates, []string{"progressive metal"}, rand.New(rand.NewSource(1)))
	second := diversifyDiscoveryCandidatesWithRand(candidates, []string{"progressive metal"}, rand.New(rand.NewSource(2)))
	if first[0].Artist != "Tie One" && first[0].Artist != "Tie Two" {
		t.Fatalf("first diversified candidate = %#v, want a strongest-fit candidate", first[0])
	}
	if second[0].Artist != "Tie One" && second[0].Artist != "Tie Two" {
		t.Fatalf("second diversified candidate = %#v, want a strongest-fit candidate", second[0])
	}
	if first[0].Artist == second[0].Artist && first[1].Artist == second[1].Artist {
		t.Fatalf("seeded tie ordering did not vary: first=%#v second=%#v", first, second)
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

func TestGetVerifiedDiscoveryCandidatesFiltersRomanizedAlbumAliases(t *testing.T) {
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
		"不恰好な街と僕と君",
		"Bukakkou na Machi to Boku to Kimi",
	} {
		t.Run(exclusion, func(t *testing.T) {
			exclusions := database.NewAlbumExclusionSet()
			if err := exclusions.Add(candidate.Artist, exclusion); err != nil {
				t.Fatalf("failed to add exclusion: %v", err)
			}
			candidates, err := getVerifiedDiscoveryCandidates(
				context.Background(),
				discoverySourceFunc(func(context.Context, []string, int) ([]DiscoveryCandidate, error) {
					return []DiscoveryCandidate{candidate}, nil
				}),
				[]string{"math rock"},
				1,
				exclusions,
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

// seedAwareDiscoveryFake records whether the seeded or tag-only search path was
// used, letting tests assert the dispatch and seed plumbing.
type seedAwareDiscoveryFake struct {
	searchCalls int
	seededCalls int
	gotTags     []string
	gotSeeds    []string
}

func (f *seedAwareDiscoveryFake) Search(_ context.Context, searchTags []string, _ int) ([]DiscoveryCandidate, error) {
	f.searchCalls++
	f.gotTags = append([]string(nil), searchTags...)
	return []DiscoveryCandidate{
		{TrackName: "TagTrack", Artist: "TagArtist", Album: "TagAlbum", Runtime: "3:00", ReleaseYear: 2020},
	}, nil
}

func (f *seedAwareDiscoveryFake) SearchWithSeeds(_ context.Context, searchTags, seedArtists []string, _ int) ([]DiscoveryCandidate, error) {
	f.seededCalls++
	f.gotTags = append([]string(nil), searchTags...)
	f.gotSeeds = append([]string(nil), seedArtists...)
	return []DiscoveryCandidate{
		{TrackName: "SeedTrack", Artist: "SeedArtist", Album: "SeedAlbum", Runtime: "3:01", ReleaseYear: 2021},
	}, nil
}

func TestGetVerifiedDiscoveryCandidatesUsesSeedsWhenSourceSupportsThem(t *testing.T) {
	// Without seed artists the dispatcher falls back to the tag-only search.
	fake := &seedAwareDiscoveryFake{}
	if _, err := getVerifiedDiscoveryCandidates(
		context.Background(), fake, []string{"math rock"}, 5, database.NewAlbumExclusionSet(),
	); err != nil {
		t.Fatalf("getVerifiedDiscoveryCandidates() error = %v", err)
	}
	if fake.seededCalls != 0 || fake.searchCalls != 1 {
		t.Fatalf("seededCalls = %d, searchCalls = %d; want tag-only search", fake.seededCalls, fake.searchCalls)
	}

	// With seed artists the seeded path is used, deduplicating and normalizing.
	fake = &seedAwareDiscoveryFake{}
	candidates, err := getVerifiedDiscoveryCandidatesSeeded(
		context.Background(), fake, []string{"math rock"}, []string{"Don Caballero", "Don Caballero"}, 5, database.NewAlbumExclusionSet(),
	)
	if err != nil {
		t.Fatalf("getVerifiedDiscoveryCandidatesSeeded() error = %v", err)
	}
	if fake.seededCalls != 1 || fake.searchCalls != 0 {
		t.Fatalf("seededCalls = %d, searchCalls = %d; want seeded search", fake.seededCalls, fake.searchCalls)
	}
	if len(fake.gotSeeds) != 1 || fake.gotSeeds[0] != "Don Caballero" {
		t.Fatalf("gotSeeds = %#v, want a single deduplicated seed", fake.gotSeeds)
	}
	if len(candidates) != 1 || candidates[0].TrackName != "SeedTrack" {
		t.Fatalf("candidates = %#v, want the seeded candidate", candidates)
	}
}

func TestMusicBrainzDiscoverySearchWithSeedsMergesArtistAndTagResults(t *testing.T) {
	sourceServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/release-group/") {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"genres":[],"tags":[]}`))
			return
		}
		if request.URL.Path != "/recording" {
			t.Errorf("request path = %q, want /recording", request.URL.Path)
		}
		query := request.URL.Query().Get("query")
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(query, `artist:"Autechre"`):
			_, _ = writer.Write([]byte(`{
				"recordings": [{
					"title": "Gantz Graf", "length": 238000, "first-release-date": "2002-08-05",
					"artist-credit": [{"name": "Autechre"}],
					"releases": [{
						"title": "Gantz Graf", "status": "Official", "date": "2002-08-05",
						"release-group": {"id": "rg-artist", "primary-type": "Album"}
					}]
				}]
			}`))
		case strings.Contains(query, `tag:"idm"`):
			_, _ = writer.Write([]byte(`{
				"recordings": [{
					"title": "Albert", "length": 322000, "first-release-date": "2018-08-07",
					"artist-credit": [{"name": "Aphex Twin"}],
					"releases": [{
						"title": "Collapse", "status": "Official", "date": "2018-09-14",
						"release-group": {"id": "rg-tag", "primary-type": "Album"}
					}]
				}]
			}`))
		default:
			_, _ = writer.Write([]byte(`{"recordings":[]}`))
		}
	}))
	defer sourceServer.Close()

	source := newMusicBrainzDiscoveryClient(musicBrainzDiscoveryConfig{
		HTTPClient:   sourceServer.Client(),
		BaseURL:      sourceServer.URL,
		UserAgent:    "discovery-test/1.0",
		Timeout:      time.Second,
		RequestDelay: 0,
	})

	candidates, err := source.SearchWithSeeds(context.Background(), []string{"idm"}, []string{"Autechre"}, 10)
	if err != nil {
		t.Fatalf("SearchWithSeeds() error = %v", err)
	}

	artistTrack := false
	tagTrack := false
	for _, candidate := range candidates {
		switch candidate.TrackName {
		case "Gantz Graf":
			artistTrack = true
		case "Albert":
			tagTrack = true
		}
	}
	if !artistTrack || !tagTrack {
		t.Fatalf("candidates = %#v; want both artist-anchored and tag results", candidates)
	}
}

func TestMusicBrainzTagNamesSortsDedupesAndLimits(t *testing.T) {
	tags := []musicBrainzTag{
		{Name: "idm", Count: 5},
		{Name: "  ", Count: 99},
		{Name: "Math Rock", Count: 20},
		{Name: "math rock", Count: 1},
		{Name: "breakcore", Count: 12},
		{Name: "electronic", Count: 3},
		{Name: "experimental", Count: 2},
	}

	got := musicBrainzTagNames(tags, 3)
	want := []string{"Math Rock", "breakcore", "idm"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("musicBrainzTagNames() = %#v, want %#v", got, want)
	}
}

func TestMusicBrainzDiscoveryUsesReleaseGroupGenres(t *testing.T) {
	requestCount := 0
	sourceServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount++
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/recording":
			_, _ = writer.Write([]byte(`{
				"recordings": [{
					"title": "Omen",
					"length": 152000,
					"first-release-date": "2016-07-15",
					"artist-credit": [{"name": "Savant"}],
					"genres": [{"name": "melodic death metal", "count": 9}],
					"releases": [{
						"title": "Vybz",
						"status": "Official",
						"date": "2016-07-15",
						"release-group": {"id": "release-group-electronic", "primary-type": "Album"}
					}]
				}]
			}`))
		case "/release-group/release-group-electronic":
			if got := request.URL.Query().Get("inc"); got != "genres+tags" {
				t.Errorf("release-group inc = %q, want genres+tags", got)
			}
			_, _ = writer.Write([]byte(`{
				"genres": [{"name": "electronic", "count": 12}, {"name": "experimental", "count": 3}]
			}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer sourceServer.Close()

	source := newMusicBrainzDiscoveryClient(musicBrainzDiscoveryConfig{
		HTTPClient:   sourceServer.Client(),
		BaseURL:      sourceServer.URL,
		UserAgent:    "discovery-test/1.0",
		Timeout:      time.Second,
		RequestDelay: 0,
	})
	candidates, err := source.Search(context.Background(), []string{"power metal"}, 20)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("MusicBrainz request count = %d, want recording plus release-group lookup", requestCount)
	}
	if len(candidates) != 0 {
		t.Fatalf("Search() returned %#v, want the recording-level metal false positive removed", candidates)
	}
}

func TestMusicBrainzDiscoveryPrefersReleaseGroupGenresInCandidate(t *testing.T) {
	sourceServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/recording":
			_, _ = writer.Write([]byte(`{
				"recordings": [{
					"title": "Lead Work",
					"length": 240000,
					"first-release-date": "2020-01-01",
					"artist-credit": [{"name": "Example Band"}],
					"genres": [{"name": "melodic death metal", "count": 9}],
					"releases": [{
						"title": "Example Album",
						"status": "Official",
						"date": "2020-01-01",
						"release-group": {"id": "release-group-power", "primary-type": "Album"}
					}]
				}]
			}`))
		case "/release-group/release-group-power":
			_, _ = writer.Write([]byte(`{
				"genres": [{"name": "power metal", "count": 12}]
			}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer sourceServer.Close()

	source := newMusicBrainzDiscoveryClient(musicBrainzDiscoveryConfig{
		HTTPClient:   sourceServer.Client(),
		BaseURL:      sourceServer.URL,
		Timeout:      time.Second,
		RequestDelay: 0,
	})
	candidates, err := source.Search(context.Background(), []string{"power metal"}, 20)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(Search()) = %d, want 1", len(candidates))
	}
	if got, want := candidates[0].GenreTags, []string{"power metal"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("GenreTags = %#v, want album-level genres %#v", got, want)
	}
}

func TestMusicBrainzDiscoverySearchSongsSkipsReleaseGroupLookup(t *testing.T) {
	var requestCount int
	sourceServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount++
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path != "/recording" {
			t.Fatalf("request path = %q, want /recording", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"recordings":[{
			"title":"Song Result","length":180000,"first-release-date":"2020-01-01",
			"artist-credit":[{"name":"Example Artist"}],
			"releases":[{"title":"Example Album","status":"Official","date":"2020-01-01",
			"release-group":{"id":"release-group-song","primary-type":"Album"}}]
		}]}`))
	}))
	defer sourceServer.Close()

	source := newMusicBrainzDiscoveryClient(musicBrainzDiscoveryConfig{
		HTTPClient: sourceServer.Client(),
		BaseURL:    sourceServer.URL,
		Timeout:    time.Second,
	})
	candidates, err := source.SearchSongs(context.Background(), []string{"power metal"}, nil, 5)
	if err != nil {
		t.Fatalf("SearchSongs() error = %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("MusicBrainz request count = %d, want recording search only", requestCount)
	}
	if len(candidates) != 1 || candidates[0].TrackName != "Song Result" {
		t.Fatalf("SearchSongs() = %#v, want one verified song candidate", candidates)
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
