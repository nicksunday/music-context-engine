package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nicksunday/music-context-platform/internal/database"
)

func TestReleaseTasteMetalNeighbors(t *testing.T) {
	for _, seed := range []struct{ artist, genre string }{
		{"Symphony X", "progressive metal"}, {"Alestorm", "folk metal"},
		{"Ensiferum", "viking metal"}, {"Rhapsody of Fire", "symphonic metal"},
	} {
		t.Run(seed.artist, func(t *testing.T) {
			reason := releaseTasteReason([]string{"metal", "power metal", "symphonic metal"}, []string{seed.genre})
			if reason == "" || !strings.Contains(reason, seed.genre) {
				t.Fatalf("reason = %q for %s", reason, seed.artist)
			}
		})
	}
	for _, tags := range [][]string{{"metal"}, {"rock"}, {"american"}, {"deathcore"}, {"hip hop"}} {
		if reason := releaseTasteReason(tags, []string{"progressive metal"}); reason != "" {
			t.Fatalf("unrelated tags %v matched: %s", tags, reason)
		}
	}
}

func TestReleaseRadarDiscoversUntaggedAlbumFromArtistTaste(t *testing.T) {
	db := openWebTestDB(t)
	_, err := db.Ctx.Exec(`INSERT INTO tracks (id,title,album,artist,clean_title,clean_artist,genres,is_favorite) VALUES ('taste-track','Song','Album','Ensiferum','song','ensiferum','["folk metal"]',1)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Ctx.Exec(`INSERT INTO tracks (id,title,album,artist,clean_title,clean_artist,genres,is_disliked) VALUES ('disliked-track','Song','Album','Disliked','song','disliked','[]',1)`)
	if err != nil {
		t.Fatal(err)
	}
	const artistID = "85997032-ff48-4383-b86a-a920d4230a0b"
	var lookups int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/explore/fresh-releases":
			fmt.Fprint(w, `{"payload":{"releases":[
    {"artist_credit_name":"Gloryhammer","artist_mbids":["`+artistID+`"],"release_name":"Space 1993: Wrath of Kor-Virliath","release_date":"2026-10-16","release_group_primary_type":"Album","release_tags":[]},
    {"artist_credit_name":"Disliked","release_name":"Excluded","release_date":"2026-10-16","release_group_primary_type":"Album","release_tags":["folk metal"]},
    {"artist_credit_name":"Unrelated","release_name":"No connection","release_date":"2026-10-16","release_group_primary_type":"Album","release_tags":["metal"]},
    {"artist_credit_name":"Future","release_name":"Tomorrow","release_date":"2026-10-17","release_group_primary_type":"Album","release_tags":["folk metal"]},
    {"artist_credit_name":"Another Discovery","release_name":"Tagged album","release_date":"2026-10-15","release_group_primary_type":"Album","release_tags":["folk metal"]}
   ]}}`)
		case "/artist":
			lookups++
			if r.URL.Query().Get("query") != "arid:"+artistID {
				t.Errorf("query = %q", r.URL.Query().Get("query"))
			}
			fmt.Fprint(w, `{"artists":[{"id":"`+artistID+`","tags":[{"name":"power metal","count":10},{"name":"symphonic metal","count":2}]}]}`)
		default:
			t.Errorf("unexpected lookup %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	radar := (&MusicBrainzReleaseRadar{baseURL: server.URL, listenBrainzURL: server.URL, client: server.Client()}).WithTasteProfile(db.Ctx)
	until := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	releases, err := radar.NewReleases(context.Background(), []database.ArtistAffinity{{Artist: "Ensiferum"}}, until.AddDate(0, 0, -90), until, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 2 || releases[0].Artist != "Gloryhammer" || lookups != 1 {
		t.Fatalf("releases = %#v; lookups = %d", releases, lookups)
	}
	if !strings.Contains(releases[0].MatchReason, "folk metal") || !strings.Contains(releases[0].MatchReason, "adjacent") {
		t.Fatalf("missing explanation: %#v", releases[0])
	}
}

func TestReleaseArtistTagLookupsAreBoundedAndPartial(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, "unavailable", 503)
			return
		}
		terms := strings.Split(r.URL.Query().Get("query"), " OR ")
		if len(terms) > 100 {
			t.Errorf("unbounded query: %d", len(terms))
		}
		var artists []map[string]any
		for _, term := range terms {
			artists = append(artists, map[string]any{"id": strings.TrimPrefix(term, "arid:"), "tags": []map[string]any{{"name": "power metal", "count": 1}}})
		}
		json.NewEncoder(w).Encode(map[string]any{"artists": artists})
	}))
	defer server.Close()
	radar := &MusicBrainzReleaseRadar{baseURL: server.URL, client: server.Client()}
	var releases []listenBrainzFreshRelease
	for i := 0; i < 405; i++ {
		releases = append(releases, listenBrainzFreshRelease{ArtistCreditName: fmt.Sprintf("Artist %d", i), ArtistMBIDs: []string{fmt.Sprintf("00000000-0000-0000-0000-%012d", i)}, ReleaseDate: "2026-10-16", ReleaseGroupPrimaryType: "Album"})
	}
	until := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	tags, err := radar.freshArtistTags(context.Background(), releases, until.AddDate(0, 0, -90), until, []string{"folk metal"}, nil)
	if err == nil || calls != 4 || len(tags) != 300 {
		t.Fatalf("tags=%d calls=%d error=%v", len(tags), calls, err)
	}
}

func TestReleaseRadarArtistMetadataFailureKeepsKnownReleases(t *testing.T) {
	db := openWebTestDB(t)
	_, err := db.Ctx.Exec(`INSERT INTO tracks (id,title,album,artist,clean_title,clean_artist,genres,is_favorite) VALUES ('taste','Song','Album','Liked','song','liked','["folk metal"]',1)`)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/artist" {
			http.Error(w, "unavailable", 503)
			return
		}
		fmt.Fprint(w, `{"payload":{"releases":[
   {"artist_credit_name":"Liked","release_name":"Known album","release_date":"2026-10-16","release_group_primary_type":"Album","release_tags":["folk metal"]},
   {"artist_credit_name":"Unknown","artist_mbids":["85997032-ff48-4383-b86a-a920d4230a0b"],"release_name":"Unknown album","release_date":"2026-10-16","release_group_primary_type":"Album"}
  ]}}`)
	}))
	defer server.Close()
	radar := (&MusicBrainzReleaseRadar{baseURL: server.URL, listenBrainzURL: server.URL, client: server.Client()}).WithTasteProfile(db.Ctx)
	until := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	releases, err := radar.NewReleases(context.Background(), []database.ArtistAffinity{{Artist: "Liked"}}, until.AddDate(0, 0, -90), until, 100)
	if err == nil || len(releases) != 1 || releases[0].Artist != "Liked" {
		t.Fatalf("releases=%#v error=%v", releases, err)
	}
}
