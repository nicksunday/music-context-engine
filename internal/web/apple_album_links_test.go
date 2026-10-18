package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicksunday/music-context-platform/internal/database"
)

func TestAppleAlbumLinksReportedCases(t *testing.T) {
	for _, example := range []struct {
		artist, album, destination string
		artistID                   int64
		appleTitle                 string
	}{
		{"Strawberry Girls", "VEGA", "https://music.apple.com/us/album/vega-single/6816842927", 0, "VEGA - Single"},
		{"Tropical Fuck Storm", "Inflatable Graveyard, Volume 2", "https://music.apple.com/us/album/inflatable-graveyard-vol-2/6777963523", 1274275070, "Inflatable Graveyard Vol. 2"},
	} {
		t.Run(example.artist, func(t *testing.T) {
			searchCalls, pageCalls := 0, 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/search" {
					searchCalls++
					if r.URL.Query().Get("entity") != "album" {
						t.Error("not an album search")
					}
					if example.artistID != 0 {
						// The new album is absent from iTunes, but the artist's older catalog
						// provides the exact artist identity needed for the public catalog.
						fmt.Fprintf(w, `{"results":[{"artistName":%q,"artistId":%d,"collectionName":"Submersive Behaviour"}]}`, example.artist, example.artistID)
					} else {
						fmt.Fprintf(w, `{"results":[{"artistName":%q,"collectionName":%q,"collectionViewUrl":%q}]}`, example.artist, example.appleTitle, example.destination)
					}
					return
				}
				if r.URL.Path != "/us/artist/1274275070" {
					t.Errorf("unexpected catalog %s", r.URL.Path)
				}
				pageCalls++
				fmt.Fprintf(w, `<a href="https://music.apple.com/us/album/unrelated/1">Unrelated</a><a class="title" href="%s"><span>%s</span></a>`, example.destination, example.appleTitle)
			}))
			defer upstream.Close()
			linker := &AppleMusicLinker{baseURL: upstream.URL + "/search", catalogBaseURL: upstream.URL, client: upstream.Client()}
			server := &Server{linkResolver: linker}
			for i := 0; i < 2; i++ {
				response := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodGet, "/api/releases/open", nil)
				query := request.URL.Query()
				query.Set("artist", example.artist)
				query.Set("album", example.album)
				request.URL.RawQuery = query.Encode()
				server.handleOpenRelease(response, request)
				if response.Code != http.StatusFound || response.Header().Get("Location") != example.destination {
					t.Fatalf("redirect = %d %q", response.Code, response.Header().Get("Location"))
				}
			}
			if example.artistID != 0 && (searchCalls != 3 || pageCalls != 1) {
				t.Fatalf("search=%d catalog=%d; cached click should make no lookups", searchCalls, pageCalls)
			}
			if example.artistID == 0 && searchCalls != 1 {
				t.Fatalf("cache missed: %d calls", searchCalls)
			}
		})
	}
}

func TestAppleAlbumTitleMatchingKeepsDistinctRecordings(t *testing.T) {
	for _, pair := range [][2]string{{"Album", "Album (Deluxe Edition)"}, {"Album", "Album [2026 Remaster]"}, {"VEGA", "VEGA - Single"}, {"Inflatable Graveyard, Volume 2", "Inflatable Graveyard Vol. 2"}} {
		if appleAlbumMatchScore(pair[0], pair[1]) == 0 {
			t.Errorf("did not match %v", pair)
		}
	}
	for _, pair := range [][2]string{{"Album", "Album (Live)"}, {"Album", "Tribute to Album"}, {"VEGA", "VEGA 2"}, {"Inflatable Graveyard Vol. 2", "Inflatable Graveyard Vol. 1"}} {
		if appleAlbumMatchScore(pair[0], pair[1]) != 0 {
			t.Errorf("incorrect match %v", pair)
		}
	}
}

func TestAppleAlbumFallbackQueriesAndRejectsWrongArtist(t *testing.T) {
	var queries []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		term := r.URL.Query().Get("term")
		queries = append(queries, term)
		if term != "album" {
			fmt.Fprint(w, `{"results":[]}`)
			return
		}
		fmt.Fprint(w, `{"results":[
   {"artistName":"Other Artist","collectionName":"Album","collectionViewUrl":"https://music.apple.com/us/album/wrong/1"},
   {"artistName":"Artist","collectionName":"Album (Deluxe Edition)","collectionViewUrl":"https://music.apple.com/us/album/deluxe/2"},
   {"artistName":"Artist","collectionName":"Album","collectionViewUrl":"https://music.apple.com/us/album/exact/3"}
  ]}`)
	}))
	defer upstream.Close()
	linker := &AppleMusicLinker{baseURL: upstream.URL, client: upstream.Client()}
	resolved, err := linker.Resolve(context.Background(), database.RecommendationCandidateInput{Artist: "Artist", Album: "Album"})
	if err != nil || !strings.HasSuffix(resolved, "/exact/3") || len(queries) != 2 {
		t.Fatalf("url=%q err=%v queries=%v", resolved, err, queries)
	}
}
