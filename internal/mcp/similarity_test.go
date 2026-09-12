package mcp

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

type fakeSimilarSource struct {
	results map[string][]similarArtist
}

func (f *fakeSimilarSource) SimilarArtists(_ context.Context, artist string, _ int) ([]similarArtist, error) {
	return f.results[artist], nil
}

func TestSimilarArtistNamesReturnsRealSimilarArtistsExcludingLibrary(t *testing.T) {
	source := &fakeSimilarSource{
		results: map[string][]similarArtist{
			"Mastodon": {{Name: "Gojira", Match: 0.9}, {Name: "Opeth", Match: 0.85}},
			"Opeth":    {{Name: "Enslaved", Match: 0.8}},
		},
	}

	// Opeth is already in the library and must be excluded; Gojira and Enslaved
	// are new-to-the-user and should be returned, deduplicated, capped to 3.
	exclude := map[string]bool{"opeth": true}
	names := SimilarArtistNames(context.Background(), source, []string{"Mastodon", "Opeth"}, exclude, 3)

	want := []string{"Gojira", "Enslaved"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("SimilarArtistNames() = %#v, want %#v", names, want)
	}
}

func TestSimilarArtistEdgesRetainsScoresAndAllReferencePaths(t *testing.T) {
	source := &fakeSimilarSource{results: map[string][]similarArtist{
		"Symphony X":        {{Name: "Dream Theater", Match: 0.91}, {Name: "Ayreon", Match: 0.73}},
		"Children of Bodom": {{Name: "Dream Theater", Match: 0.44}, {Name: "Blind Guardian", Match: 0.68}},
	}}
	edges := SimilarArtistEdges(context.Background(), source, []string{"Symphony X", "Children of Bodom"})
	if len(edges) != 4 {
		t.Fatalf("edges = %#v, want all four provider edges", edges)
	}
	paths := 0
	for _, edge := range edges {
		if edge.Neighbor == "Dream Theater" {
			paths++
		}
		if edge.Source != "last.fm" || edge.Match <= 0 || edge.ReferenceText == "" {
			t.Fatalf("edge lost source/provenance/score: %#v", edge)
		}
	}
	if paths != 2 {
		t.Fatalf("Dream Theater provenance paths = %d, want 2", paths)
	}
}

func TestSimilarArtistNamesReturnsNilWhenSourceUnconfigured(t *testing.T) {
	if got := SimilarArtistNames(context.Background(), nil, []string{"Mastodon"}, nil, 3); got != nil {
		t.Fatalf("SimilarArtistNames(nil source) = %#v, want nil", got)
	}
}

func TestSimilarArtistNamesDedupesNormalizedNamesAndHonorsCap(t *testing.T) {
	source := &fakeSimilarSource{
		results: map[string][]similarArtist{
			"Mastodon": {{Name: "Gojira", Match: 1}, {Name: "gojira", Match: 0.9}},
			"Opeth":    {{Name: "Enslaved", Match: 0.8}},
		},
	}
	names := SimilarArtistNames(context.Background(), source, []string{"Mastodon", "Opeth"}, nil, 1)
	if !reflect.DeepEqual(names, []string{"Gojira"}) {
		t.Fatalf("SimilarArtistNames(cap=1) = %#v, want [Gojira]", names)
	}
}

type trackedSimilarSource struct {
	results map[string][]similarArtist
	calls   []string
}

func (f *trackedSimilarSource) SimilarArtists(_ context.Context, artist string, limit int) ([]similarArtist, error) {
	f.calls = append(f.calls, artist)
	if limit != maxSimilarArtistsPerSeed {
		panic("unexpected lookup limit")
	}
	if artist == "failed" {
		return nil, fmt.Errorf("unavailable")
	}
	return f.results[artist], nil
}

func TestSimilarArtistNamesBalancesSeeds(t *testing.T) {
	source := &trackedSimilarSource{results: map[string][]similarArtist{
		"A": {{Name: "A1"}, {Name: "A2"}, {Name: "A3"}, {Name: "A4"}},
		"B": {{Name: "B1"}}, "C": {{Name: "C1"}}, "D": {{Name: "D1"}},
	}}
	got := SimilarArtistNames(context.Background(), source, []string{"A", " a ", "B", "C", "D", "E"}, nil, 4)
	if !reflect.DeepEqual(got, []string{"A1", "B1", "C1", "D1"}) {
		t.Fatalf("unbalanced neighbors: %v", got)
	}
	if !reflect.DeepEqual(source.calls, []string{"A", "B", "C", "D"}) {
		t.Fatalf("unbounded/duplicate calls: %v", source.calls)
	}
}

func TestSimilarArtistNamesRedistributesAndSkipsDuplicates(t *testing.T) {
	source := &trackedSimilarSource{results: map[string][]similarArtist{
		"A": {{Name: "blocked"}, {Name: "Shared"}, {Name: "A2"}},
		"B": {{Name: "shared"}, {Name: "B1"}, {Name: "B2"}},
	}}
	got := SimilarArtistNames(context.Background(), source, []string{"A", "failed", "empty", "B"}, map[string]bool{"blocked": true}, 5)
	if !reflect.DeepEqual(got, []string{"Shared", "B1", "A2", "B2"}) {
		t.Fatalf("neighbors: %v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source.calls = nil
	if got := SimilarArtistNames(ctx, source, []string{"A"}, nil, 4); len(got) != 0 || len(source.calls) != 0 {
		t.Fatal("canceled lookup ran")
	}
}
