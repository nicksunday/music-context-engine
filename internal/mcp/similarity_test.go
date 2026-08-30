package mcp

import (
	"context"
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
