package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nicksunday/music-context-platform/internal/database"
	mcpserver "github.com/nicksunday/music-context-platform/internal/mcp"
)

type fakeRecommender struct {
	draft          RecommendationDraft
	requestCapture *RecommendationRequest
}

func (fake fakeRecommender) Recommend(_ context.Context, request RecommendationRequest, _ ProfileContext) (RecommendationDraft, error) {
	if fake.requestCapture != nil {
		*fake.requestCapture = request
	}
	return fake.draft, nil
}

type fakeVerifier struct {
	allowed map[string]bool
}

type fakeLinkResolver struct {
	urls map[string]string
	errs map[string]error
}

type fakeSongLinkResolver struct {
	link SongLink
}

type fakeAppleMusicPlaylistProvider struct {
	tracks []AppleMusicTrack
	result AppleMusicPlaylistResult
	err    error
}

func (fake *fakeAppleMusicPlaylistProvider) ResolveTrack(_ context.Context, candidate database.RecommendationCandidateInput) (AppleMusicTrack, bool, error) {
	if strings.TrimSpace(candidate.Song) == "missing" {
		return AppleMusicTrack{}, false, nil
	}
	return AppleMusicTrack{Artist: candidate.Artist, Song: candidate.Song, ID: candidate.Song, URL: "https://music.apple.com/us/song/" + url.PathEscape(candidate.Song)}, true, nil
}

func (fake *fakeAppleMusicPlaylistProvider) CreatePlaylist(_ context.Context, _ string, tracks []AppleMusicTrack) (AppleMusicPlaylistResult, error) {
	fake.tracks = append([]AppleMusicTrack(nil), tracks...)
	if fake.err != nil {
		return AppleMusicPlaylistResult{}, fake.err
	}
	return fake.result, nil
}

func (fake fakeSongLinkResolver) ResolveSong(context.Context, database.RecommendationCandidateInput) (SongLink, error) {
	return fake.link, nil
}

func TestAppleMusicAppURLConvertsCanonicalDestination(t *testing.T) {
	got := AppleMusicAppURL("https://music.apple.com/us/song/example/123")
	if got != "itmss://music.apple.com/us/song/example/123" {
		t.Fatalf("AppleMusicAppURL() = %q", got)
	}
	if got := AppleMusicAppURL("https://example.com/song/123"); got != "" {
		t.Fatalf("AppleMusicAppURL(non-Apple Music URL) = %q, want empty", got)
	}
}

func TestAppleMusicConfigEndpointFailsClosedWithoutDeveloperToken(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/apple-music/config", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var payload AppleMusicConfigResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Enabled || payload.DeveloperToken != "" {
		t.Fatalf("payload = %#v, want disabled config without token", payload)
	}
}

func TestAppleMusicConfigEndpointReturnsDeveloperTokenOnlyWhenConfigured(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{AppleMusicDeveloperToken: "test-developer-token"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/apple-music/config", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var payload AppleMusicConfigResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Enabled || payload.DeveloperToken != "test-developer-token" {
		t.Fatalf("payload = %#v, want configured token", payload)
	}
}

func TestModeScopedBatchEndpointsDoNotCrossRecommendationTypes(t *testing.T) {
	db := openWebTestDB(t)
	album, err := database.CreateRecommendationBatch(context.Background(), db.Ctx, database.RecommendationBatchInput{
		Prompt: "albums", Mode: "album", Candidates: []database.RecommendationCandidateInput{{Artist: "Album Artist", Album: "Album"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.CreateRecommendationBatch(context.Background(), db.Ctx, database.RecommendationBatchInput{
		Prompt: "songs", Mode: "song", Candidates: []database.RecommendationCandidateInput{{Artist: "Song Artist", Album: "Album", Song: "Song"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := NewServer(db.Ctx, Options{})
	record := httptest.NewRecorder()
	handler.ServeHTTP(record, httptest.NewRequest(http.MethodGet, "/api/batch/latest?mode=album", nil))
	if record.Code != http.StatusOK {
		t.Fatalf("latest status = %d", record.Code)
	}
	var latest BatchResponse
	if err := json.NewDecoder(record.Body).Decode(&latest); err != nil {
		t.Fatal(err)
	}
	if latest.Batch == nil || latest.Batch.ID != album.ID {
		t.Fatalf("latest album batch = %#v", latest.Batch)
	}

	record = httptest.NewRecorder()
	handler.ServeHTTP(record, httptest.NewRequest(http.MethodGet, "/api/batches?mode=song", nil))
	if record.Code != http.StatusOK {
		t.Fatalf("sessions status = %d", record.Code)
	}
	var sessions BatchesResponse
	if err := json.NewDecoder(record.Body).Decode(&sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions.Batches) != 1 || sessions.Batches[0].Mode != "song" {
		t.Fatalf("song sessions = %#v", sessions.Batches)
	}
}

func TestRecommendationPagesExposeSharedNavigation(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{})
	for _, path := range []string{"/recommendations", "/recommendations/songs", "/recommendations/albums"} {
		record := httptest.NewRecorder()
		handler.ServeHTTP(record, httptest.NewRequest(http.MethodGet, path, nil))
		if record.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, record.Code)
		}
		body := record.Body.String()
		for _, expected := range []string{"Song Recommendations", "Album Recommendations", "/recommendations/songs", "/recommendations/albums"} {
			if !strings.Contains(body, expected) {
				t.Fatalf("GET %s body does not contain %q", path, expected)
			}
		}
	}
}

func TestRecommendationFrontendContainsModeSpecificPresentationContracts(t *testing.T) {
	for name, expected := range map[string][]string{
		"static/index.html": {"id=\"modeChooser\"", "data-mode=\"song\"", "data-mode=\"album\""},
		"static/app.js":     {"function renderSongCandidate", "Not Today", "/api/batch/latest?mode=", "/api/batches?mode=", "individual songs to hear next", "high-impact albums to check out next", "initialPrompt"},
		"static/app.css":    {".song-row", ".recommendation-nav", ".mode-choice", "grid-template-rows: auto auto auto minmax(0, 1fr)"},
	} {
		content, err := fs.ReadFile(staticFiles, name)
		if err != nil {
			t.Fatalf("read embedded %s: %v", name, err)
		}
		for _, expectedValue := range expected {
			if !strings.Contains(string(content), expectedValue) {
				t.Fatalf("embedded %s does not contain %q", name, expectedValue)
			}
		}
	}
}

func TestAppleMusicPlaylistEndpointResolvesAndDeduplicatesTracks(t *testing.T) {
	db := openWebTestDB(t)
	batch, err := database.CreateRecommendationBatch(context.Background(), db.Ctx, database.RecommendationBatchInput{
		Prompt: "individual songs", Mode: "song", Candidates: []database.RecommendationCandidateInput{
			{Artist: "Artist 1", Album: "Album 1", Song: "song-1"},
			{Artist: "Artist 2", Album: "Album 2", Song: "missing"},
			{Artist: "Artist 1", Album: "Album 1", Song: "song-1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeAppleMusicPlaylistProvider{result: AppleMusicPlaylistResult{URL: "https://music.apple.com/us/playlist/test/1", Added: 1}}
	handler := NewServer(db.Ctx, Options{AppleMusic: provider})
	record := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/playlists/apple-music", bytes.NewBufferString(`{"batch_id":"`+batch.ID+`"}`))
	handler.ServeHTTP(record, request)
	if record.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", record.Code, record.Body.String())
	}
	if len(provider.tracks) != 1 || provider.tracks[0].Song != "song-1" {
		t.Fatalf("provider tracks = %#v, want one ordered track", provider.tracks)
	}
	var response PlaylistResponse
	if err := json.NewDecoder(record.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if !response.Partial || len(response.Skipped) != 2 {
		t.Fatalf("response = %#v, want partial response with missing and duplicate skips", response)
	}
}

func (fake fakeLinkResolver) Resolve(
	_ context.Context,
	candidate database.RecommendationCandidateInput,
) (string, error) {
	cleanArtist, cleanAlbum, err := database.NormalizeAlbumLookup(candidate.Artist, candidate.Album)
	if err != nil {
		return "", err
	}
	key := cleanArtist + "/" + cleanAlbum
	if fake.errs != nil {
		if err := fake.errs[key]; err != nil {
			return "", err
		}
	}
	return fake.urls[key], nil
}

type fakeDiscoveryProvider struct {
	request    DiscoveryRequest
	candidates []mcpserver.DiscoveryCandidate
}

type fakeReleaseRadar struct {
	releases    []NewRelease
	err         error
	artistCount int
	since       time.Time
	until       time.Time
	limit       int
}

func TestClampBatchLimitUsesModeSpecificMaximums(t *testing.T) {
	tests := []struct {
		name  string
		mode  string
		input int
		want  int
	}{
		{name: "song accepts values above old maximum", mode: "song", input: 15, want: 15},
		{name: "song caps at song maximum", mode: "song", input: 25, want: maxSongBatchLimit},
		{name: "album retains album maximum", mode: "album", input: 25, want: maxAlbumBatchLimit},
		{name: "empty mode is album", mode: "", input: 25, want: maxAlbumBatchLimit},
		{name: "non-positive uses default", mode: "song", input: 0, want: defaultBatchLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampBatchLimit(tt.mode, tt.input); got != tt.want {
				t.Fatalf("clampBatchLimit(%q, %d) = %d, want %d", tt.mode, tt.input, got, tt.want)
			}
		})
	}
}

func TestDiscoveryCandidateFetchLimitUsesModeSpecificMaximums(t *testing.T) {
	if got, want := discoveryCandidateFetchLimit("song", 15), 45; got != want {
		t.Fatalf("song discovery fetch limit = %d, want %d", got, want)
	}
	if got, want := discoveryCandidateFetchLimit("album", 15), 30; got != want {
		t.Fatalf("album discovery fetch limit = %d, want %d", got, want)
	}
}

func TestRecommendationEndpointAcceptsSongLimitAboveTen(t *testing.T) {
	db := openWebTestDB(t)
	var captured RecommendationRequest
	candidates := make([]database.RecommendationCandidateInput, 15)
	for idx := range candidates {
		candidates[idx] = database.RecommendationCandidateInput{
			Artist:       "Artist " + strconv.Itoa(idx),
			Album:        "Album " + strconv.Itoa(idx),
			StarterTrack: "Track " + strconv.Itoa(idx),
			Song:         "Track " + strconv.Itoa(idx),
		}
	}
	handler := NewServer(db.Ctx, Options{
		Recommender: fakeRecommender{
			draft:          RecommendationDraft{Reply: "Try these songs.", Candidates: candidates},
			requestCapture: &captured,
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/recommendations", bytes.NewBufferString(`{"message":"give me many individual songs","mode":"song","limit":15}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	if captured.Mode != "song" || captured.Limit != 15 {
		t.Fatalf("captured request = %#v, want song mode with limit 15", captured)
	}
	var payload RecommendationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Batch.Candidates) != 15 {
		t.Fatalf("candidate count = %d, want 15", len(payload.Batch.Candidates))
	}
}

func (fake *fakeDiscoveryProvider) Discover(
	_ context.Context,
	request DiscoveryRequest,
) ([]mcpserver.DiscoveryCandidate, error) {
	fake.request = request
	return fake.candidates, nil
}

func (fake *fakeReleaseRadar) NewReleases(
	_ context.Context,
	artists []database.ArtistAffinity,
	since time.Time,
	until time.Time,
	limit int,
) ([]NewRelease, error) {
	fake.artistCount = len(artists)
	fake.since = since
	fake.until = until
	fake.limit = limit
	return fake.releases, fake.err
}

func (fake fakeVerifier) Verify(
	_ context.Context,
	candidate database.RecommendationCandidateInput,
) (database.RecommendationCandidateInput, bool, error) {
	cleanArtist, cleanAlbum, err := database.NormalizeAlbumLookup(candidate.Artist, candidate.Album)
	if err != nil {
		return candidate, false, err
	}
	return candidate, fake.allowed[cleanArtist+"/"+cleanAlbum], nil
}

func TestMCPGroundedRecommenderUsesVerifiedDiscoveryCandidates(t *testing.T) {
	callCount := 0
	ollama := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")

		var content string
		switch callCount {
		case 0:
			content = `{"vibe_summary":"heavy rhythmic music with low-end drive and left-field energy","required_traits":["new-to-user"],"flexible_traits":["heavy","funk/groove","experimental"],"target_vibe":"","fallback_tags":["funk metal","experimental rock"]}`
		case 1:
			content = `{"reply":"Try this verified pick.","selections":[{"candidate_index":2,"note":"The tags fit the heavy and weird side without leaning on anchors."}]}`
		default:
			t.Fatalf("unexpected Ollama call %d", callCount+1)
		}
		callCount++

		if err := json.NewEncoder(writer).Encode(ollamaChatResponse{
			Message: ollamaMessage{Content: content},
		}); err != nil {
			t.Fatalf("failed to encode fake Ollama response: %v", err)
		}
	}))
	defer ollama.Close()

	discovery := &fakeDiscoveryProvider{candidates: []mcpserver.DiscoveryCandidate{
		{
			Artist:      "Praxis",
			Album:       "Transmutation (Mutatis Mutandis)",
			TrackName:   "Animal Behavior",
			Runtime:     "4:18",
			ReleaseYear: 1992,
			GenreTags:   []string{"funk metal", "experimental rock"},
		},
		{
			Artist:      "Clown Core",
			Album:       "Van",
			TrackName:   "Computers",
			Runtime:     "1:48",
			ReleaseYear: 2020,
			GenreTags:   []string{"experimental rock"},
		},
	}}
	recommender := newMCPGroundedOllamaRecommenderWithDiscovery(ollama.URL, "fake", time.Minute, discovery)

	draft, err := recommender.Recommend(context.Background(), RecommendationRequest{
		Message: "heavy, funk, weird",
		Limit:   1,
	}, ProfileContext{})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}
	if callCount != 2 {
		t.Fatalf("Ollama call count = %d, want 2", callCount)
	}
	if len(discovery.request.FallbackTags) != 2 ||
		discovery.request.FallbackTags[0] != "funk metal" ||
		discovery.request.FallbackTags[1] != "experimental rock" {
		t.Fatalf("discovery fallback tags = %#v, want planned tags", discovery.request.FallbackTags)
	}
	if len(draft.Candidates) != 1 {
		t.Fatalf("len(draft.Candidates) = %d, want 1", len(draft.Candidates))
	}
	candidate := draft.Candidates[0]
	if candidate.Artist != "Clown Core" || candidate.Album != "Van" || candidate.StarterTrack != "Computers" {
		t.Fatalf("candidate = %#v, want selected verified MCP candidate", candidate)
	}
}

func TestSimilarSeedArtistsWithoutSourceFallsBackToNil(t *testing.T) {
	recommender := newMCPGroundedOllamaRecommenderWithDiscovery(
		"", "fake", time.Minute, &fakeDiscoveryProvider{},
	)
	got := recommender.similarSeedArtists(context.Background(), []database.ArtistAffinity{
		{Artist: "Mastodon"},
	})
	if got != nil {
		t.Fatalf("similarSeedArtists() = %#v, want nil when no similar source configured", got)
	}
}

func TestPromptTraitCoverageFlagsUnsupportedFunk(t *testing.T) {
	request := RecommendationRequest{
		Message: "heavy, weird, funky",
	}

	supported, unsupported := promptTraitCoverage(request, mcpserver.DiscoveryCandidate{
		GenreTags: []string{"progressive metal", "technical death metal"},
	})
	if containsString(supported, "funk/groove") {
		t.Fatalf("supported = %#v, did not expect funk/groove for technical death metal tags", supported)
	}
	if !containsString(unsupported, "funk/groove") {
		t.Fatalf("unsupported = %#v, want funk/groove", unsupported)
	}
	if !containsString(supported, "heavy") {
		t.Fatalf("supported = %#v, want heavy", supported)
	}

	supported, unsupported = promptTraitCoverage(request, mcpserver.DiscoveryCandidate{
		GenreTags: []string{"funk metal", "experimental rock"},
	})
	if !containsString(supported, "funk/groove") || !containsString(supported, "weird/experimental") || !containsString(supported, "heavy") {
		t.Fatalf("supported = %#v, want funk/groove, weird/experimental, and heavy", supported)
	}
	if len(unsupported) != 0 {
		t.Fatalf("unsupported = %#v, want none", unsupported)
	}
}

func TestPromptTraitCoverageSupportsBassLineViaGrooveProxy(t *testing.T) {
	request := RecommendationRequest{
		Message: "great bass lines and heavy or funky or both",
	}

	supported, unsupported := promptTraitCoverage(request, mcpserver.DiscoveryCandidate{
		GenreTags: []string{"funk rock", "groove metal"},
	})
	if !containsString(supported, "bass/rhythm") {
		t.Fatalf("supported = %#v, want bass/rhythm", supported)
	}
	if containsString(unsupported, "bass/rhythm") {
		t.Fatalf("unsupported = %#v, did not expect bass/rhythm", unsupported)
	}

	supported, unsupported = promptTraitCoverage(request, mcpserver.DiscoveryCandidate{
		GenreTags: []string{"technical death metal", "progressive metal"},
	})
	if containsString(supported, "bass/rhythm") {
		t.Fatalf("supported = %#v, did not expect bass/rhythm", supported)
	}
	if !containsString(unsupported, "bass/rhythm") {
		t.Fatalf("unsupported = %#v, want bass/rhythm", unsupported)
	}
}

func TestPromptTraitCoverageFlagsVirtuosicLeadPlaying(t *testing.T) {
	request := RecommendationRequest{
		Message: "I'd like new-to-me metal albums with virtuosic playing like Symphony X or Children of Bodom",
		Mood:    "melodic, heavy",
	}

	supported, unsupported := promptTraitCoverage(request, mcpserver.DiscoveryCandidate{
		GenreTags: []string{"melodic death metal", "death metal"},
	})
	if containsString(supported, "virtuosic/lead-playing") {
		t.Fatalf("supported = %#v, did not expect broad death metal to support virtuosic playing", supported)
	}
	if !containsString(unsupported, "virtuosic/lead-playing") {
		t.Fatalf("unsupported = %#v, want virtuosic/lead-playing", unsupported)
	}
	if !containsString(supported, "melodic") || !containsString(supported, "heavy") {
		t.Fatalf("supported = %#v, want melodic and heavy", supported)
	}

	supported, unsupported = promptTraitCoverage(request, mcpserver.DiscoveryCandidate{
		GenreTags: []string{"progressive metal", "power metal", "neoclassical metal"},
	})
	if !containsString(supported, "virtuosic/lead-playing") ||
		!containsString(supported, "melodic") ||
		!containsString(supported, "heavy") {
		t.Fatalf("supported = %#v, want virtuosic/lead-playing, melodic, and heavy", supported)
	}
	if len(unsupported) != 0 {
		t.Fatalf("unsupported = %#v, want none", unsupported)
	}
}

func TestPromptTraitLogicDetectsOrRequests(t *testing.T) {
	logic := promptTraitLogic(RecommendationRequest{
		Message: "great bass lines and heavy or funky or both",
	})
	if !strings.Contains(logic, "any-of") {
		t.Fatalf("promptTraitLogic() = %q, want any-of", logic)
	}

	logic = promptTraitLogic(RecommendationRequest{
		Message: "heavy, funky, weird",
	})
	if !strings.Contains(logic, "multi-trait") {
		t.Fatalf("promptTraitLogic() = %q, want multi-trait", logic)
	}
}

func TestDiscoveryPlanCalibrationRemovesMetalWhenPromptIsNonMetal(t *testing.T) {
	plan := calibrateDiscoveryPlanForPrompt(
		RecommendationRequest{Message: "I want eclectic, bass forward, jazz bands in the style of KNOWER"},
		modelDiscoveryPlan{
			FallbackTags: []string{"funk metal", "groove metal", "progressive metal"},
		},
	)
	for _, tag := range plan.FallbackTags {
		if strings.Contains(tag, "metal") {
			t.Fatalf("FallbackTags = %#v, did not expect metal tag for non-metal KNOWER prompt", plan.FallbackTags)
		}
	}
	for _, want := range []string{"jazz-funk", "jazz fusion", "electropop", "funk"} {
		if !containsString(plan.FallbackTags, want) {
			t.Fatalf("FallbackTags = %#v, want %q", plan.FallbackTags, want)
		}
	}
}

func TestDiscoveryPlanCalibrationKeepsMetalWhenPromptRequestsHeavy(t *testing.T) {
	plan := calibrateDiscoveryPlanForPrompt(
		RecommendationRequest{Message: "I want heavy, funky, weird albums"},
		modelDiscoveryPlan{FallbackTags: []string{"funk metal", "groove metal", "experimental rock"}},
	)
	if !containsString(plan.FallbackTags, "funk metal") {
		t.Fatalf("FallbackTags = %#v, want funk metal retained for heavy prompt", plan.FallbackTags)
	}
}

func TestDiscoveryPlanCalibrationRemovesAvoidedMetalTags(t *testing.T) {
	plan := calibrateDiscoveryPlanForPrompt(
		RecommendationRequest{
			Message: "I'd like some new-to-me metal albums that feature vituosic playing like Symphony X or Children of Bodom.",
			Mood:    "melodic, heavy",
			Avoid:   "avante garde metal, technical death metal",
		},
		modelDiscoveryPlan{
			TargetVibe:   "technical death metal",
			FallbackTags: []string{"technical death metal", "avant-garde metal", "progressive metal", "melodic death metal", "power metal"},
		},
	)
	for _, blocked := range []string{"technical death metal", "avant-garde metal"} {
		if containsString(plan.FallbackTags, blocked) {
			t.Fatalf("FallbackTags = %#v, did not expect avoided tag %q", plan.FallbackTags, blocked)
		}
	}
	if plan.TargetVibe != "" {
		t.Fatalf("TargetVibe = %q, want avoided target vibe cleared", plan.TargetVibe)
	}
	for _, want := range []string{"neoclassical metal", "power metal", "progressive metal", "symphonic metal", "melodic metal", "melodic death metal"} {
		if !containsString(plan.FallbackTags, want) {
			t.Fatalf("FallbackTags = %#v, want %q retained", plan.FallbackTags, want)
		}
	}

	plan = calibrateDiscoveryPlanForPrompt(
		RecommendationRequest{
			Message: "I'd like some new-to-me metal albums that feature vituosic playing like Symphony X or Children of Bodom.",
			Mood:    "melodic, heavy",
			Avoid:   "technical death metal",
		},
		modelDiscoveryPlan{FallbackTags: []string{"technical death metal"}},
	)
	if containsString(plan.FallbackTags, "technical death metal") {
		t.Fatalf("FallbackTags = %#v, did not expect avoided tag after derived fallback", plan.FallbackTags)
	}
	for _, want := range []string{"progressive metal", "power metal", "melodic metal"} {
		if !containsString(plan.FallbackTags, want) {
			t.Fatalf("FallbackTags = %#v, want derived fallback tag %q", plan.FallbackTags, want)
		}
	}
}

func TestDiscoveryPlanCalibrationPrependsVirtuosicAnchorTags(t *testing.T) {
	plan := calibrateDiscoveryPlanForPrompt(
		RecommendationRequest{
			Message: "I'd like some new-to-me metal albums that feature vituosic playing like Symphony X or Children of Bodom.",
			Mood:    "melodic, heavy",
			Avoid:   "technical death metal",
		},
		modelDiscoveryPlan{
			FallbackTags: []string{"melodic death metal", "progressive metal"},
		},
	)

	wantPrefix := []string{"neoclassical metal", "power metal", "progressive metal", "symphonic metal", "melodic metal"}
	if len(plan.FallbackTags) < len(wantPrefix) {
		t.Fatalf("FallbackTags = %#v, want virtuosic anchor prefix", plan.FallbackTags)
	}
	for idx, want := range wantPrefix {
		if plan.FallbackTags[idx] != want {
			t.Fatalf("FallbackTags = %#v, tag %d = %q, want %q", plan.FallbackTags, idx, plan.FallbackTags[idx], want)
		}
	}
	if containsString(plan.FallbackTags, "technical death metal") {
		t.Fatalf("FallbackTags = %#v, did not expect avoided technical death metal", plan.FallbackTags)
	}
}

func TestRankDiscoveryCandidatesForVirtuosicMetalPrioritizesAnchorTags(t *testing.T) {
	request := RecommendationRequest{
		Message: "I'd like some new-to-me metal albums that feature vituosic playing like Symphony X or Children of Bodom.",
		Mood:    "melodic, heavy",
		Avoid:   "technical death metal",
	}
	plan := modelDiscoveryPlan{
		FallbackTags: []string{"neoclassical metal", "power metal", "progressive metal", "symphonic metal", "melodic metal", "melodic death metal"},
	}

	ranked := rankDiscoveryCandidatesForPrompt(request, plan, []mcpserver.DiscoveryCandidate{
		{
			Artist:    "Broad Melodeath",
			Album:     "Heavy Enough",
			GenreTags: []string{"melodic death metal", "death metal"},
		},
		{
			Artist:    "Adagio",
			Album:     "Underworld",
			GenreTags: []string{"progressive metal", "power metal", "neoclassical metal"},
		},
		{
			Artist:    "Extreme Tech",
			Album:     "Blast Index",
			GenreTags: []string{"technical death metal", "deathcore"},
		},
	})

	if ranked[0].Artist != "Adagio" {
		t.Fatalf("ranked candidates = %#v, want Adagio first", ranked)
	}
	if ranked[len(ranked)-1].Artist != "Extreme Tech" {
		t.Fatalf("ranked candidates = %#v, want avoided extreme technical death last", ranked)
	}
}

func TestDiscoveryPlanCalibrationAddsComparisonFields(t *testing.T) {
	plan := calibrateDiscoveryPlanForPrompt(
		RecommendationRequest{Message: "Give me Rage Against the Machine vibes"},
		modelDiscoveryPlan{},
	)

	for _, want := range []string{"rage against the machine"} {
		if !containsString(plan.ReferenceAnchors, want) {
			t.Fatalf("ReferenceAnchors = %#v, want %q", plan.ReferenceAnchors, want)
		}
	}
	for _, want := range []string{"funk metal", "rap metal", "rhythmic groove", "staccato riffs"} {
		if !containsString(plan.ComparisonTraits, want) {
			t.Fatalf("ComparisonTraits = %#v, want %q", plan.ComparisonTraits, want)
		}
	}
	if !containsString(plan.FalseFriendTraits, "generic heavy metal") {
		t.Fatalf("FalseFriendTraits = %#v, want generic heavy metal", plan.FalseFriendTraits)
	}
	if !containsString(plan.BridgeTraits, "alternative metal") {
		t.Fatalf("BridgeTraits = %#v, want alternative metal", plan.BridgeTraits)
	}
	for _, want := range []string{"funk metal", "rap metal", "alternative metal", "funk rock"} {
		if !containsString(plan.FallbackTags, want) {
			t.Fatalf("FallbackTags = %#v, want %q", plan.FallbackTags, want)
		}
	}

	plan = calibrateDiscoveryPlanForPrompt(
		RecommendationRequest{Message: "Something like KNOWER but heavier"},
		modelDiscoveryPlan{},
	)
	if !containsString(plan.ReferenceAnchors, "knower") {
		t.Fatalf("ReferenceAnchors = %#v, want knower", plan.ReferenceAnchors)
	}
	if !containsString(plan.ComparisonModifiers, "heavier") {
		t.Fatalf("ComparisonModifiers = %#v, want heavier", plan.ComparisonModifiers)
	}
	if !containsString(plan.ComparisonTraits, "jazz-funk") || !containsString(plan.ComparisonTraits, "electronic fusion") {
		t.Fatalf("ComparisonTraits = %#v, want jazz-funk and electronic fusion", plan.ComparisonTraits)
	}
	if !containsString(plan.BridgeTraits, "added weight") {
		t.Fatalf("BridgeTraits = %#v, want added weight", plan.BridgeTraits)
	}
}

func TestRankDiscoveryCandidatesForRATMComparisonPreservesGroove(t *testing.T) {
	request := RecommendationRequest{Message: "Give me Rage Against the Machine vibes"}
	plan := calibrateDiscoveryPlanForPrompt(request, modelDiscoveryPlan{})

	ranked := rankDiscoveryCandidatesForPrompt(request, plan, []mcpserver.DiscoveryCandidate{
		{
			Artist:    "Generic Heavy",
			Album:     "Big Riffs",
			GenreTags: []string{"heavy metal"},
		},
		{
			Artist:    "Groove Target",
			Album:     "Pocket Riffs",
			GenreTags: []string{"funk metal", "rap metal", "alternative metal"},
		},
	})

	if ranked[0].Artist != "Groove Target" {
		t.Fatalf("ranked candidates = %#v, want groove/rap-metal candidate first", ranked)
	}
}

func TestRankDiscoveryCandidatesForKNOWERButHeavierPreservesFusion(t *testing.T) {
	request := RecommendationRequest{Message: "Something like KNOWER but heavier"}
	plan := calibrateDiscoveryPlanForPrompt(request, modelDiscoveryPlan{})

	ranked := rankDiscoveryCandidatesForPrompt(request, plan, []mcpserver.DiscoveryCandidate{
		{
			Artist:    "Generic Sludge",
			Album:     "Weight Only",
			GenreTags: []string{"sludge metal", "heavy metal"},
		},
		{
			Artist:    "Fusion Weight",
			Album:     "Odd Meter Dance",
			GenreTags: []string{"jazz-funk", "electro-funk", "funk metal"},
		},
	})

	if ranked[0].Artist != "Fusion Weight" {
		t.Fatalf("ranked candidates = %#v, want jazz-funk/electronic fusion candidate first", ranked)
	}
}

func TestRankDiscoveryCandidatesForOpethLessDeathMetalRespectsSubtractiveConstraint(t *testing.T) {
	request := RecommendationRequest{Message: "Something like Opeth but less death metal"}
	plan := calibrateDiscoveryPlanForPrompt(request, modelDiscoveryPlan{})

	ranked := rankDiscoveryCandidatesForPrompt(request, plan, []mcpserver.DiscoveryCandidate{
		{
			Artist:    "Death Focus",
			Album:     "Growl Wall",
			GenreTags: []string{"death metal", "progressive death metal"},
		},
		{
			Artist:    "Dynamic Prog",
			Album:     "Clean Transitions",
			GenreTags: []string{"progressive metal", "progressive rock", "atmospheric"},
		},
	})

	if ranked[0].Artist != "Dynamic Prog" {
		t.Fatalf("ranked candidates = %#v, want progressive/dynamic candidate first", ranked)
	}
}

func TestPromptTraitCoverageDoesNotTreatTechnicalityAsComparisonSupport(t *testing.T) {
	request := RecommendationRequest{
		Message: "I'd like virtuosic metal like Symphony X or Children of Bodom, especially symphonic metal",
	}
	plan := calibrateDiscoveryPlanForPrompt(request, modelDiscoveryPlan{})

	supported, unsupported := promptTraitCoverageForPlan(request, plan, mcpserver.DiscoveryCandidate{
		GenreTags: []string{"technical death metal", "deathcore"},
	})
	if containsString(supported, "virtuosic/lead-playing") {
		t.Fatalf("supported = %#v, did not expect technical death metal to support lead-playing", supported)
	}
	if !containsString(unsupported, "virtuosic/lead-playing") || !containsString(unsupported, "comparison target traits") {
		t.Fatalf("unsupported = %#v, want virtuosic/lead-playing and comparison target traits", unsupported)
	}
}

func TestFallbackDiscoveryDraftKeepsComparisonNotesGrounded(t *testing.T) {
	request := RecommendationRequest{Message: "Something like KNOWER but heavier", Limit: 2}
	plan := calibrateDiscoveryPlanForPrompt(request, modelDiscoveryPlan{})

	draft := fallbackDiscoveryDraft(request, []mcpserver.DiscoveryCandidate{
		{
			Artist:      "Generic Heavy",
			Album:       "Weight Only",
			TrackName:   "Mass",
			Runtime:     "4:00",
			ReleaseYear: 2020,
			GenreTags:   []string{"heavy metal"},
		},
		{
			Artist:      "Fusion Weight",
			Album:       "Odd Meter Dance",
			TrackName:   "Pocket",
			Runtime:     "3:22",
			ReleaseYear: 2021,
			GenreTags:   []string{"jazz-funk", "funk metal"},
		},
	}, plan, 2)

	if len(draft.Candidates) != 2 {
		t.Fatalf("len(draft.Candidates) = %d, want 2", len(draft.Candidates))
	}
	if strings.Contains(strings.ToLower(draft.Candidates[0].Note), "knower") ||
		strings.Contains(strings.ToLower(draft.Candidates[0].Note), "jazz-funk") {
		t.Fatalf("generic candidate note = %q, should not claim unsupported KNOWER/jazz-funk similarity", draft.Candidates[0].Note)
	}
	if !strings.Contains(draft.Candidates[1].Note, "jazz-funk") {
		t.Fatalf("bridge/supported candidate note = %q, want supported jazz-funk evidence", draft.Candidates[1].Note)
	}
}

func TestFallbackDiscoveryDraftDefaultsToOneAlbumPerArtist(t *testing.T) {
	request := RecommendationRequest{Message: "weird progressive metal", Limit: 3}
	plan := modelDiscoveryPlan{FallbackTags: []string{"progressive metal"}}

	draft := fallbackDiscoveryDraft(request, []mcpserver.DiscoveryCandidate{
		{Artist: "Mono Band", Album: "First Album", TrackName: "First", Runtime: "3:01", ReleaseYear: 2020},
		{Artist: "Mono Band", Album: "Second Album", TrackName: "Second", Runtime: "3:02", ReleaseYear: 2021},
		{Artist: "Other Project", Album: "Other Album", TrackName: "Other", Runtime: "3:03", ReleaseYear: 2022},
	}, plan, 3)

	if len(draft.Candidates) != 2 {
		t.Fatalf("len(draft.Candidates) = %d, want one Mono Band album plus Other Project", len(draft.Candidates))
	}
	if draft.Candidates[0].Album != "First Album" || draft.Candidates[1].Artist != "Other Project" {
		t.Fatalf("draft.Candidates = %#v, want first Mono Band album then Other Project", draft.Candidates)
	}
	if draft.Candidates[0].Rank != 1 || draft.Candidates[1].Rank != 2 {
		t.Fatalf("candidate ranks = %d/%d, want 1/2", draft.Candidates[0].Rank, draft.Candidates[1].Rank)
	}
}

func TestAvoidedTagFilteringDropsCandidatesWithAvoidedGenres(t *testing.T) {
	avoidTags := parseAvoidTags("avante garde metal; tech death")
	discoveryCandidates := filterAvoidedDiscoveryCandidates([]mcpserver.DiscoveryCandidate{
		{Artist: "Changeling", Album: "Changeling", GenreTags: []string{"progressive metal", "technical death metal"}},
		{Artist: "Ashenspire", Album: "Hostile Architecture", GenreTags: []string{"progressive metal", "avant-garde metal"}},
		{Artist: "Kamelot", Album: "The Fourth Legacy", GenreTags: []string{"power metal", "symphonic metal"}},
	}, avoidTags)
	if len(discoveryCandidates) != 1 || discoveryCandidates[0].Artist != "Kamelot" {
		t.Fatalf("filtered discovery candidates = %#v, want only non-avoided candidate", discoveryCandidates)
	}

	recommendationCandidates := filterAvoidedRecommendationCandidates([]database.RecommendationCandidateInput{
		{Artist: "Obsidious", Album: "Iconic", GenreTags: []string{"progressive technical death metal"}, Rank: 4},
		{Artist: "Adagio", Album: "Underworld", GenreTags: []string{"progressive metal", "power metal"}, Rank: 5},
	}, avoidTags)
	if len(recommendationCandidates) != 1 || recommendationCandidates[0].Artist != "Adagio" {
		t.Fatalf("filtered recommendation candidates = %#v, want only non-avoided candidate", recommendationCandidates)
	}
	if recommendationCandidates[0].Rank != 1 {
		t.Fatalf("filtered recommendation rank = %d, want rank reset to 1", recommendationCandidates[0].Rank)
	}
}

func TestMCPGroundedRecommenderFiltersAvoidedDiscoveryCandidates(t *testing.T) {
	callCount := 0
	ollama := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")

		var content string
		switch callCount {
		case 0:
			content = `{"vibe_summary":"melodic virtuosic metal","required_traits":["new-to-user"],"flexible_traits":["melodic","heavy","virtuosic"],"target_vibe":"","fallback_tags":["technical death metal","progressive metal","power metal"]}`
		case 1:
			content = `{"reply":"Try the melodic power-metal pick.","selections":[{"candidate_index":1,"note":"Keeps the melodic, virtuosic side without avoided tags."}]}`
		default:
			t.Fatalf("unexpected Ollama call %d", callCount+1)
		}
		callCount++

		if err := json.NewEncoder(writer).Encode(ollamaChatResponse{
			Message: ollamaMessage{Content: content},
		}); err != nil {
			t.Fatalf("failed to encode fake Ollama response: %v", err)
		}
	}))
	defer ollama.Close()

	discovery := &fakeDiscoveryProvider{candidates: []mcpserver.DiscoveryCandidate{
		{
			Artist:      "Changeling",
			Album:       "Changeling",
			TrackName:   "Cathexis Interlude",
			Runtime:     "4:19",
			ReleaseYear: 2024,
			GenreTags:   []string{"progressive metal", "technical death metal"},
		},
		{
			Artist:      "Adagio",
			Album:       "Underworld",
			TrackName:   "Next Profundis",
			Runtime:     "7:39",
			ReleaseYear: 2003,
			GenreTags:   []string{"progressive metal", "power metal"},
		},
	}}
	recommender := newMCPGroundedOllamaRecommenderWithDiscovery(ollama.URL, "fake", time.Minute, discovery)

	draft, err := recommender.Recommend(context.Background(), RecommendationRequest{
		Message: "I'd like some new-to-me metal albums that feature vituosic playing like Symphony X or Children of Bodom.",
		Mood:    "melodic, heavy",
		Avoid:   "technical death metal",
		Limit:   1,
	}, ProfileContext{})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}
	if containsString(discovery.request.FallbackTags, "technical death metal") {
		t.Fatalf("discovery fallback tags = %#v, did not expect avoided tag", discovery.request.FallbackTags)
	}
	if len(draft.Candidates) != 1 {
		t.Fatalf("len(draft.Candidates) = %d, want 1", len(draft.Candidates))
	}
	if draft.Candidates[0].Artist != "Adagio" || draft.Candidates[0].Album != "Underworld" {
		t.Fatalf("candidate = %#v, want non-avoided discovery candidate", draft.Candidates[0])
	}
}

func TestDiscoverySelectionPromptIncludesInterpretedVibe(t *testing.T) {
	prompt := discoverySelectionUserPrompt(
		RecommendationRequest{Message: "great bass lines and heavy or funky or both", Limit: 3},
		ProfileContext{},
		modelDiscoveryPlan{
			VibeSummary:         "prominent low-end drive with either weight or groove",
			RequiredTraits:      []string{"new-to-user"},
			FlexibleTraits:      []string{"bass/rhythm", "heavy", "funk/groove"},
			ReferenceAnchors:    []string{"rage against the machine"},
			ComparisonTraits:    []string{"funk metal", "rap metal", "rhythmic groove"},
			FalseFriendTraits:   []string{"generic heavy metal"},
			BridgeTraits:        []string{"alternative metal"},
			ComparisonModifiers: []string{"more groove-focused"},
			FallbackTags:        []string{"funk metal", "groove metal", "post-punk"},
		},
		[]mcpserver.DiscoveryCandidate{
			{
				Artist:      "Example Artist",
				Album:       "Example Album",
				TrackName:   "Example Track",
				Runtime:     "3:21",
				ReleaseYear: 2020,
				GenreTags:   []string{"groove metal"},
			},
		},
		3,
	)

	for _, want := range []string{
		"Interpreted vibe: prominent low-end drive with either weight or groove",
		"Required traits: new-to-user",
		"Flexible traits: bass/rhythm, heavy, funk/groove",
		"Comparison anchors: rage against the machine",
		"Comparison traits: funk metal, rap metal, rhythmic groove",
		"False-friend traits: generic heavy metal",
		"Bridge traits: alternative metal",
		"Comparison modifiers: more groove-focused",
		"Prompt trait logic: any-of",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("selection prompt missing %q:\n%s", want, prompt)
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestContextEndpointReturnsProfile(t *testing.T) {
	db := openWebTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres, user_rating, track_count)
		VALUES ('album-1', 'Dos City', 'Dos Monos', 'dos city', 'dos monos', '["experimental hip hop"]', 4.3, 13);
		INSERT INTO tracks (id, album_id, title, album, artist, clean_title, clean_artist, genres, is_favorite)
		VALUES ('track-1', 'album-1', 'In 20XX', 'Dos City', 'Dos Monos', 'in 20xx', 'dos monos', '["experimental hip hop"]', 1);`)
	if err != nil {
		t.Fatalf("failed to insert profile fixture: %v", err)
	}

	handler := NewServer(db.Ctx, Options{Model: "test-model", OllamaURL: "http://localhost:11434"})
	request := httptest.NewRequest(http.MethodGet, "/api/context", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	var payload ContextResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode context response: %v", err)
	}
	if payload.Model != "test-model" {
		t.Fatalf("payload.Model = %q, want test-model", payload.Model)
	}
	if len(payload.Artists) == 0 || payload.Artists[0].Artist != "Dos Monos" {
		t.Fatalf("payload.Artists = %#v, want Dos Monos", payload.Artists)
	}
}

func TestBatchHistoryEndpointsReadLatestAndSessions(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/batch/latest", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("empty latest status = %d, want 200", response.Code)
	}
	var emptyPayload BatchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &emptyPayload); err != nil {
		t.Fatalf("decode empty latest response: %v", err)
	}
	if emptyPayload.Batch != nil {
		t.Fatalf("empty latest batch = %#v, want nil", emptyPayload.Batch)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/batches", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("empty sessions status = %d, want 200", response.Code)
	}
	var emptySessions BatchesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &emptySessions); err != nil {
		t.Fatalf("decode empty sessions response: %v", err)
	}
	if emptySessions.Batches == nil || len(emptySessions.Batches) != 0 {
		t.Fatalf("empty sessions = %#v, want empty array", emptySessions.Batches)
	}

	first, err := database.CreateRecommendationBatch(context.Background(), db.Ctx, database.RecommendationBatchInput{
		Prompt: "first prompt",
		Mood:   "focused",
		Notes:  "first reply",
		Candidates: []database.RecommendationCandidateInput{
			{Artist: "First Artist", Album: "First Album", StarterTrack: "First Track"},
		},
	})
	if err != nil {
		t.Fatalf("create first batch: %v", err)
	}
	second, err := database.CreateRecommendationBatch(context.Background(), db.Ctx, database.RecommendationBatchInput{
		Prompt: "second prompt",
		Mood:   "energetic",
		Notes:  "second reply",
		Candidates: []database.RecommendationCandidateInput{
			{Artist: "Second Artist", Album: "Second Album", StarterTrack: "Second Track"},
		},
	})
	if err != nil {
		t.Fatalf("create second batch: %v", err)
	}

	var beforeCount int
	if err := db.Ctx.QueryRow("SELECT COUNT(*) FROM recommendation_batches").Scan(&beforeCount); err != nil {
		t.Fatalf("count batches before reads: %v", err)
	}
	var beforeCandidates int
	if err := db.Ctx.QueryRow("SELECT COUNT(*) FROM recommendation_candidates").Scan(&beforeCandidates); err != nil {
		t.Fatalf("count candidates before reads: %v", err)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/batch/latest", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("latest status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var latestPayload BatchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &latestPayload); err != nil {
		t.Fatalf("decode latest response: %v", err)
	}
	if latestPayload.Batch == nil || latestPayload.Batch.ID != second.ID || len(latestPayload.Batch.Candidates) != 1 {
		t.Fatalf("latest payload = %#v, want second batch", latestPayload.Batch)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/batch?id="+first.ID, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("by-id status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var byIDPayload BatchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &byIDPayload); err != nil {
		t.Fatalf("decode by-id response: %v", err)
	}
	if byIDPayload.Batch == nil || byIDPayload.Batch.ID != first.ID || byIDPayload.Batch.Candidates[0].Rank != 1 {
		t.Fatalf("by-id payload = %#v, want first batch with rank", byIDPayload.Batch)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/batch?id=missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing by-id status = %d, want 404", response.Code)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/batches", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("sessions status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var sessionsPayload BatchesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &sessionsPayload); err != nil {
		t.Fatalf("decode sessions response: %v", err)
	}
	if len(sessionsPayload.Batches) != 2 || sessionsPayload.Batches[0].ID != second.ID || sessionsPayload.Batches[1].ID != first.ID {
		t.Fatalf("sessions = %#v, want newest first", sessionsPayload.Batches)
	}
	if sessionsPayload.Batches[0].CandidateCount != 1 || sessionsPayload.Batches[0].Reply != "second reply" {
		t.Fatalf("latest session summary = %#v, want count/reply", sessionsPayload.Batches[0])
	}

	var afterCount, afterCandidates int
	if err := db.Ctx.QueryRow("SELECT COUNT(*) FROM recommendation_batches").Scan(&afterCount); err != nil {
		t.Fatalf("count batches after reads: %v", err)
	}
	if err := db.Ctx.QueryRow("SELECT COUNT(*) FROM recommendation_candidates").Scan(&afterCandidates); err != nil {
		t.Fatalf("count candidates after reads: %v", err)
	}
	if afterCount != beforeCount || afterCandidates != beforeCandidates {
		t.Fatalf("read endpoints mutated rows: batches %d->%d, candidates %d->%d", beforeCount, afterCount, beforeCandidates, afterCandidates)
	}
}

func TestRailEndpointReturnsNewReleasesForAffinityArtists(t *testing.T) {
	db := openWebTestDB(t)
	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres, track_count)
		VALUES ('album-1', 'How Are You? We Are Fine, Thank You', 'The Down Troddence', 'how are you we are fine thank you', 'the down troddence', '["progressive metal"]', 8);
		INSERT INTO tracks (id, album_id, title, album, artist, clean_title, clean_artist, genres, is_favorite)
		VALUES ('track-1', 'album-1', 'Shiva', 'How Are You? We Are Fine, Thank You', 'The Down Troddence', 'shiva', 'the down troddence', '["progressive metal"]', 1);`)
	if err != nil {
		t.Fatalf("failed to insert affinity fixture: %v", err)
	}

	releaseRadar := &fakeReleaseRadar{releases: []NewRelease{
		{Artist: "The Down Troddence", Album: "New Heavy Thing", ReleaseDate: "2026-08-01", ReleaseType: "Album"},
	}}
	handler := NewServer(db.Ctx, Options{
		ReleaseRadar: releaseRadar,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/rail", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var payload RailResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode rail response: %v", err)
	}
	if len(payload.NewReleases) != 1 || payload.NewReleases[0].Album != "New Heavy Thing" {
		t.Fatalf("payload.NewReleases = %#v, want New Heavy Thing", payload.NewReleases)
	}
	if releaseRadar.artistCount == 0 {
		t.Fatalf("releaseRadar.artistCount = 0, want affinity artists passed to release provider")
	}
	if releaseRadar.limit != defaultRadarReleases {
		t.Fatalf("releaseRadar.limit = %d, want %d", releaseRadar.limit, defaultRadarReleases)
	}
	if gotDays := int(releaseRadar.until.Sub(releaseRadar.since).Hours() / 24); gotDays != newReleaseWindowDays {
		t.Fatalf("release window = %d days, want %d", gotDays, newReleaseWindowDays)
	}
}

func TestRailEndpointReturnsFriendlyErrorWhenNewReleaseLookupFails(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{
		ReleaseRadar: &fakeReleaseRadar{err: errors.New("upstream unavailable")},
	})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/rail", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var payload RailResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode rail response: %v", err)
	}
	if payload.ReleaseError == "" {
		t.Fatalf("payload.ReleaseError = empty, want friendly error")
	}
}

func TestRailEndpointReturnsPartialNewReleasesWithWarning(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{
		ReleaseRadar: &fakeReleaseRadar{
			releases: []NewRelease{
				{Artist: "King Gizzard & the Lizard Wizard", Album: "Alien Metal", Blurb: "Tagged electronic."},
			},
			err: errors.New("some upstream requests failed"),
		},
	})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/rail", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var payload RailResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode rail response: %v", err)
	}
	if len(payload.NewReleases) != 1 || payload.NewReleases[0].Blurb != "Tagged electronic." {
		t.Fatalf("payload.NewReleases = %#v, want partial release", payload.NewReleases)
	}
	if !strings.Contains(payload.ReleaseError, "partial") {
		t.Fatalf("payload.ReleaseError = %q, want partial warning", payload.ReleaseError)
	}
}

func TestRecommendationEndpointPersistsBatch(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{
		Recommender: fakeRecommender{draft: RecommendationDraft{
			Reply: "Try this batch.",
			Candidates: []database.RecommendationCandidateInput{
				{
					Artist:       "doseone & Steel Tipped Dove",
					Album:        "All Portrait, No Chorus",
					StarterTrack: "Wasteland Embrace",
					GenreTags:    []string{"abstract hip hop"},
					Note:         "Dense and personality-forward.",
				},
			},
		}},
		Model:     "fake",
		OllamaURL: "http://localhost:11434",
	})

	requestBody := bytes.NewBufferString(`{"message":"weird hip hop","limit":4}`)
	request := httptest.NewRequest(http.MethodPost, "/api/recommendations", requestBody)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	var payload RecommendationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode recommendation response: %v", err)
	}
	if payload.Batch.ID == "" || len(payload.Batch.Candidates) != 1 {
		t.Fatalf("payload.Batch = %#v, want one persisted candidate", payload.Batch)
	}

	var count int
	if err := db.Ctx.QueryRow("SELECT COUNT(*) FROM recommendation_candidates WHERE batch_id = ?", payload.Batch.ID).Scan(&count); err != nil {
		t.Fatalf("failed to count candidates: %v", err)
	}
	if count != 1 {
		t.Fatalf("candidate count = %d, want 1", count)
	}
}

func TestRecommendationEndpointDefaultsToOneAlbumPerArtist(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{
		Recommender: fakeRecommender{draft: RecommendationDraft{
			Reply: "Try this batch.",
			Candidates: []database.RecommendationCandidateInput{
				{Artist: "King Gizzard & The Lizard Wizard", Album: "Polygondwanaland", StarterTrack: "Crumbling Castle"},
				{Artist: "King Gizzard and the Lizard Wizard", Album: "Nonagon Infinity", StarterTrack: "Robot Stop"},
				{Artist: "Kamelot", Album: "The Black Halo", StarterTrack: "March of Mephisto"},
			},
		}},
	})

	requestBody := bytes.NewBufferString(`{"message":"melodic progressive metal","limit":3}`)
	request := httptest.NewRequest(http.MethodPost, "/api/recommendations", requestBody)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var payload RecommendationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode recommendation response: %v", err)
	}
	if len(payload.Batch.Candidates) != 2 {
		t.Fatalf("payload.Batch.Candidates = %#v, want one King Gizzard album plus Kamelot", payload.Batch.Candidates)
	}
	if payload.Batch.Candidates[0].Album != "Polygondwanaland" || payload.Batch.Candidates[1].Artist != "Kamelot" {
		t.Fatalf("payload.Batch.Candidates = %#v, want first King Gizzard album then Kamelot", payload.Batch.Candidates)
	}
	if payload.Batch.Candidates[0].Rank != 1 || payload.Batch.Candidates[1].Rank != 2 {
		t.Fatalf("candidate ranks = %d/%d, want 1/2", payload.Batch.Candidates[0].Rank, payload.Batch.Candidates[1].Rank)
	}
}

func TestRecommendationEndpointAllowsRepeatedArtistsForDeepDiveButDeduplicatesAlbums(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{
		Recommender: fakeRecommender{draft: RecommendationDraft{
			Reply: "Try this batch.",
			Candidates: []database.RecommendationCandidateInput{
				{Artist: "King Gizzard & The Lizard Wizard", Album: "Polygondwanaland", StarterTrack: "Crumbling Castle"},
				{Artist: "King Gizzard and the Lizard Wizard", Album: "Polygondwanaland", StarterTrack: "Inner Cell"},
				{Artist: "King Gizzard & The Lizard Wizard", Album: "Nonagon Infinity", StarterTrack: "Robot Stop"},
				{Artist: "Kamelot", Album: "The Black Halo", StarterTrack: "March of Mephisto"},
			},
		}},
	})

	requestBody := bytes.NewBufferString(`{"message":"deep dive into King Gizzard's catalog with multiple albums by the same artist","limit":4}`)
	request := httptest.NewRequest(http.MethodPost, "/api/recommendations", requestBody)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var payload RecommendationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode recommendation response: %v", err)
	}
	if len(payload.Batch.Candidates) != 3 {
		t.Fatalf("payload.Batch.Candidates = %#v, want duplicate album removed but repeated artist allowed", payload.Batch.Candidates)
	}
	if payload.Batch.Candidates[0].Album != "Polygondwanaland" ||
		payload.Batch.Candidates[1].Album != "Nonagon Infinity" ||
		payload.Batch.Candidates[2].Album != "The Black Halo" {
		t.Fatalf("payload.Batch.Candidates = %#v, want unique albums in original order", payload.Batch.Candidates)
	}
}

func TestFeedbackEndpointPersistsVerdict(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{})

	requestBody := bytes.NewBufferString(`{"artist":"Jungle","album":"Sunshine","verdict":"ok","notes":"mood dependent"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/feedback", requestBody)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	var verdict string
	if err := db.Ctx.QueryRow("SELECT verdict FROM recommendation_feedback WHERE clean_artist = 'jungle' AND clean_title = 'sunshine'").Scan(&verdict); err != nil {
		t.Fatalf("failed to query persisted feedback: %v", err)
	}
	if verdict != "ok" {
		t.Fatalf("verdict = %q, want ok", verdict)
	}
}

func TestRecommendationEndpointRejectsRecentFeedbackRepeats(t *testing.T) {
	db := openWebTestDB(t)
	if _, err := database.LogRecommendationFeedback(context.Background(), db.Ctx, database.RecommendationFeedbackInput{
		Artist:  "King Gizzard & The Lizard Wizard",
		Album:   "Alien Metal",
		Verdict: "good",
	}); err != nil {
		t.Fatalf("failed to insert feedback fixture: %v", err)
	}

	handler := NewServer(db.Ctx, Options{
		Recommender: fakeRecommender{draft: RecommendationDraft{
			Reply: "Try this batch.",
			Candidates: []database.RecommendationCandidateInput{
				{Artist: "King Gizzard & The Lizard Wizard", Album: "Alien Metal", StarterTrack: "The Lurkers"},
			},
		}},
	})

	requestBody := bytes.NewBufferString(`{"message":"high impact","limit":1}`)
	request := httptest.NewRequest(http.MethodPost, "/api/recommendations", requestBody)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body = %s", response.Code, response.Body.String())
	}
}

func TestRecommendationEndpointAllowsKnownArtistUnratedAlbum(t *testing.T) {
	db := openWebTestDB(t)
	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres, track_count)
		VALUES ('album-sabbath', 'Master of Reality', 'Black Sabbath', 'master of reality', 'black sabbath', '[]', 8)`)
	if err != nil {
		t.Fatalf("failed to insert local album fixture: %v", err)
	}

	handler := NewServer(db.Ctx, Options{
		Recommender: fakeRecommender{draft: RecommendationDraft{
			Reply: "Try this batch.",
			Candidates: []database.RecommendationCandidateInput{
				{Artist: "Black Sabbath", Album: "Volume 4", StarterTrack: "War Pigs"},
			},
		}},
	})

	requestBody := bytes.NewBufferString(`{"message":"heavy funk weird","limit":1}`)
	request := httptest.NewRequest(http.MethodPost, "/api/recommendations", requestBody)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	var payload RecommendationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode recommendation response: %v", err)
	}
	if len(payload.Batch.Candidates) != 1 || payload.Batch.Candidates[0].Album != "Volume 4" {
		t.Fatalf("payload.Batch.Candidates = %#v, want Volume 4", payload.Batch.Candidates)
	}
}

func TestRecommendationEndpointRejectsNumericRatedAlbum(t *testing.T) {
	db := openWebTestDB(t)
	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, user_rating)
		VALUES ('album-sabbath', 'Master of Reality', 'Black Sabbath', 'master of reality', 'black sabbath', 4.5)`)
	if err != nil {
		t.Fatalf("failed to insert rated album fixture: %v", err)
	}

	handler := NewServer(db.Ctx, Options{
		Recommender: fakeRecommender{draft: RecommendationDraft{
			Reply: "Try this batch.",
			Candidates: []database.RecommendationCandidateInput{
				{Artist: "Black Sabbath", Album: "Master of Reality", StarterTrack: "Sweet Leaf"},
			},
		}},
	})

	requestBody := bytes.NewBufferString(`{"message":"heavy funk weird","limit":1}`)
	request := httptest.NewRequest(http.MethodPost, "/api/recommendations", requestBody)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body = %s", response.Code, response.Body.String())
	}
}

func TestRecommendationEndpointTreatsNotTodayAsSameDayCooldown(t *testing.T) {
	db := openWebTestDB(t)
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1).UTC().Format("2006-01-02 15:04:05")
	if _, err := database.LogRecommendationFeedback(context.Background(), db.Ctx, database.RecommendationFeedbackInput{
		Artist:  "Today Artist",
		Album:   "Today Album",
		Verdict: "not_for_me_today",
	}); err != nil {
		t.Fatalf("failed to insert same-day not-today fixture: %v", err)
	}
	if _, err := db.Ctx.Exec(`
		INSERT INTO recommendation_feedback (id, artist, album, clean_artist, clean_title, verdict, created_at)
		VALUES ('feedback-yesterday', 'Yesterday Artist', 'Yesterday Album', 'yesterday artist', 'yesterday album', 'not_for_me_today', ?)`,
		yesterday,
	); err != nil {
		t.Fatalf("failed to insert older not-today fixture: %v", err)
	}

	handler := NewServer(db.Ctx, Options{
		Recommender: fakeRecommender{draft: RecommendationDraft{
			Reply: "Try this batch.",
			Candidates: []database.RecommendationCandidateInput{
				{Artist: "Today Artist", Album: "Today Album", StarterTrack: "Today Track"},
				{Artist: "Yesterday Artist", Album: "Yesterday Album", StarterTrack: "Yesterday Track"},
			},
		}},
	})

	requestBody := bytes.NewBufferString(`{"message":"try these again","limit":2}`)
	request := httptest.NewRequest(http.MethodPost, "/api/recommendations", requestBody)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var payload RecommendationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode recommendation response: %v", err)
	}
	if len(payload.Batch.Candidates) != 1 ||
		payload.Batch.Candidates[0].Artist != "Yesterday Artist" ||
		payload.Batch.Candidates[0].Album != "Yesterday Album" {
		t.Fatalf("payload.Batch.Candidates = %#v, want only older not-today album", payload.Batch.Candidates)
	}
}

func TestRecommendationEndpointSkipsUnverifiedCandidates(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{
		Recommender: fakeRecommender{draft: RecommendationDraft{
			Reply: "Try this batch.",
			Candidates: []database.RecommendationCandidateInput{
				{Artist: "Mastodon", Album: "Hades", StarterTrack: "Crown of Thorns"},
				{Artist: "Valid Artist", Album: "Valid Album", StarterTrack: "Valid Track"},
			},
		}},
		Verifier: fakeVerifier{allowed: map[string]bool{
			"valid artist/valid album": true,
		}},
	})

	requestBody := bytes.NewBufferString(`{"message":"heavy funk weird","limit":2}`)
	request := httptest.NewRequest(http.MethodPost, "/api/recommendations", requestBody)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	var payload RecommendationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode recommendation response: %v", err)
	}
	if len(payload.Batch.Candidates) != 1 || payload.Batch.Candidates[0].Album != "Valid Album" {
		t.Fatalf("payload.Batch.Candidates = %#v, want only Valid Album", payload.Batch.Candidates)
	}
}

func TestRecommendationEndpointResolvesAndPersistsStreamingURLs(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{
		Recommender: fakeRecommender{draft: RecommendationDraft{
			Reply: "Try this batch.",
			Candidates: []database.RecommendationCandidateInput{
				{Artist: "Jungle", Album: "Volcano", StarterTrack: "Candle Flame"},
			},
		}},
		LinkResolver: fakeLinkResolver{urls: map[string]string{
			"jungle/volcano": "https://music.apple.com/us/album/volcano/1693279903",
		}},
	})

	requestBody := bytes.NewBufferString(`{"message":"weird funk","limit":4}`)
	request := httptest.NewRequest(http.MethodPost, "/api/recommendations", requestBody)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	var payload RecommendationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode recommendation response: %v", err)
	}
	if len(payload.Batch.Candidates) != 1 {
		t.Fatalf("payload.Batch.Candidates = %#v, want one candidate", payload.Batch.Candidates)
	}

	const wantURL = "https://music.apple.com/us/album/volcano/1693279903"
	if payload.Batch.Candidates[0].StreamingURL != wantURL {
		t.Fatalf("candidate.StreamingURL = %q, want %q", payload.Batch.Candidates[0].StreamingURL, wantURL)
	}
	if !strings.Contains(response.Body.String(), `"streaming_url":"`+wantURL+`"`) {
		t.Fatalf("response body does not include %q", wantURL)
	}

	var persisted string
	if err := db.Ctx.QueryRow("SELECT streaming_url FROM recommendation_candidates WHERE batch_id = ?", payload.Batch.ID).Scan(&persisted); err != nil {
		t.Fatalf("failed to query persisted streaming_url: %v", err)
	}
	if persisted != wantURL {
		t.Fatalf("persisted streaming_url = %q, want %q", persisted, wantURL)
	}
}

func TestRecommendationEndpointSupportsSongMode(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{
		Recommender: fakeRecommender{draft: RecommendationDraft{
			Reply: "Try these songs.",
			Candidates: []database.RecommendationCandidateInput{{
				Artist: "Jungle", Album: "Volcano", StarterTrack: "Candle Flame",
			}},
		}},
		SongLinkResolver: fakeSongLinkResolver{link: SongLink{
			URL: "https://www.youtube.com/results?search_query=Jungle+Candle+Flame", Provider: "youtube",
		}},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/recommendations", bytes.NewBufferString(`{"message":"give me individual songs","mode":"song","limit":1}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var payload RecommendationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Batch.Mode != "song" || payload.Batch.Candidates[0].Song != "Candle Flame" {
		t.Fatalf("song batch = %#v, want song mode and Candle Flame", payload.Batch)
	}
	if payload.Batch.Candidates[0].StreamingProvider != "youtube" {
		t.Fatalf("provider = %q, want youtube", payload.Batch.Candidates[0].StreamingProvider)
	}
}

func TestRecommendationEndpointDegradesWhenLinkLookupFails(t *testing.T) {
	db := openWebTestDB(t)
	handler := NewServer(db.Ctx, Options{
		Recommender: fakeRecommender{draft: RecommendationDraft{
			Reply: "Try this batch.",
			Candidates: []database.RecommendationCandidateInput{
				{Artist: "Jungle", Album: "Volcano", StarterTrack: "Candle Flame"},
				{Artist: "Dos Monos", Album: "Dos City!!!", StarterTrack: "In 20XX"},
			},
		}},
		LinkResolver: fakeLinkResolver{
			errs: map[string]error{
				"jungle/volcano": errors.New("iTunes unavailable"),
			},
		},
	})

	requestBody := bytes.NewBufferString(`{"message":"weird funk","limit":4}`)
	request := httptest.NewRequest(http.MethodPost, "/api/recommendations", requestBody)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	var payload RecommendationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode recommendation response: %v", err)
	}
	if len(payload.Batch.Candidates) != 2 {
		t.Fatalf("payload.Batch.Candidates = %#v, want two candidates", payload.Batch.Candidates)
	}
	if payload.Batch.Candidates[0].StreamingURL != "" {
		t.Fatalf("candidate with failing lookup has StreamingURL = %q, want empty", payload.Batch.Candidates[0].StreamingURL)
	}
	if payload.Batch.Candidates[1].StreamingURL != "" {
		t.Fatalf("candidate with no match has StreamingURL = %q, want empty", payload.Batch.Candidates[1].StreamingURL)
	}
	if strings.Contains(response.Body.String(), `"streaming_url"`) {
		t.Fatalf("response body contains streaming_url for unresolved candidates: %s", response.Body.String())
	}

	var count int
	if err := db.Ctx.QueryRow("SELECT COUNT(*) FROM recommendation_candidates WHERE batch_id = ?", payload.Batch.ID).Scan(&count); err != nil {
		t.Fatalf("failed to count persisted candidates: %v", err)
	}
	if count != 2 {
		t.Fatalf("persisted candidate count = %d, want 2", count)
	}

	var nullCount int
	if err := db.Ctx.QueryRow("SELECT COUNT(*) FROM recommendation_candidates WHERE batch_id = ? AND streaming_url IS NULL", payload.Batch.ID).Scan(&nullCount); err != nil {
		t.Fatalf("failed to count candidates with NULL streaming_url: %v", err)
	}
	if nullCount != 2 {
		t.Fatalf("candidates with NULL streaming_url = %d, want 2", nullCount)
	}
}

func TestAppleMusicLinkerResolvesMatchingAlbum(t *testing.T) {
	iTunes := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/search" {
			http.NotFound(writer, request)
			return
		}
		if request.URL.Query().Get("entity") != "album" {
			t.Fatalf("entity query = %q, want album", request.URL.Query().Get("entity"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"resultCount": 2,
			"results": [
				{
					"artistName": "Jungle",
					"collectionName": "Volcano",
					"primaryGenreName": "Electronic",
					"collectionViewUrl": "https://music.apple.com/us/album/volcano/1693279903"
				},
				{
					"artistName": "Somebody Else",
					"collectionName": "Volcano",
					"primaryGenreName": "Rock",
					"collectionViewUrl": "https://music.apple.com/us/album/volcano-other/1"
				}
			]
		}`))
	}))
	defer iTunes.Close()

	linker := &AppleMusicLinker{
		baseURL:      iTunes.URL + "/search",
		client:       iTunes.Client(),
		requestDelay: 0,
	}

	const wantURL = "https://music.apple.com/us/album/volcano/1693279903"

	got, err := linker.Resolve(context.Background(), database.RecommendationCandidateInput{Artist: "Jungle", Album: "Volcano"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != wantURL {
		t.Fatalf("Resolve() = %q, want %q", got, wantURL)
	}

	got, err = linker.Resolve(context.Background(), database.RecommendationCandidateInput{Artist: "Somebody Else", Album: "Volcano"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "https://music.apple.com/us/album/volcano-other/1" {
		t.Fatalf("Resolve() for Somebody Else = %q, want the Somebody Else album URL", got)
	}

	got, err = linker.Resolve(context.Background(), database.RecommendationCandidateInput{Artist: "Unknown Artist", Album: "Volcano"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != "" {
		t.Fatalf("Resolve() for unknown artist = %q, want empty", got)
	}
}

func TestAppleMusicLinkerDegradesOnLookupFailures(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "boom", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	failing := &AppleMusicLinker{
		baseURL:      upstream.URL + "/search",
		client:       upstream.Client(),
		requestDelay: 0,
	}
	if _, err := failing.Resolve(context.Background(), database.RecommendationCandidateInput{Artist: "Jungle", Album: "Volcano"}); err == nil {
		t.Fatal("Resolve() with non-2xx response = nil error, want error")
	}

	unconfigured := &AppleMusicLinker{}
	if got, err := unconfigured.Resolve(context.Background(), database.RecommendationCandidateInput{Artist: "Jungle", Album: "Volcano"}); err != nil || got != "" {
		t.Fatalf("Resolve() with no client = %q, %v; want empty, nil", got, err)
	}
}

func TestAppleMusicLinkerResolvesMatchingSong(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("entity") != "song" {
			t.Fatalf("entity query = %q, want song", request.URL.Query().Get("entity"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"results":[
			{"artistName":"Jungle","trackName":"Other Song","trackViewUrl":"https://music.apple.com/us/song/other/1"},
			{"artistName":"Jungle","trackName":"Candle Flame","trackViewUrl":"https://music.apple.com/us/song/candle-flame/2"}
		]}`))
	}))
	defer upstream.Close()

	linker := &AppleMusicLinker{baseURL: upstream.URL + "/search", client: upstream.Client()}
	link, err := linker.ResolveSong(context.Background(), database.RecommendationCandidateInput{
		Artist: "Jungle",
		Song:   "Candle Flame",
	})
	if err != nil {
		t.Fatalf("ResolveSong() error = %v", err)
	}
	if link.URL != "https://music.apple.com/us/song/candle-flame/2" || link.Provider != "apple_music" {
		t.Fatalf("ResolveSong() = %#v, want matching Apple Music link", link)
	}
}

func TestFallbackSongLinkResolverUsesYouTubeWhenAppleMusicMisses(t *testing.T) {
	link, err := (FallbackSongLinkResolver{
		AppleMusic: &AppleMusicLinker{},
		YouTube:    YouTubeSongLinker{},
	}).ResolveSong(context.Background(), database.RecommendationCandidateInput{
		Artist: "Unknown Artist",
		Song:   "Rare Song",
	})
	if err != nil {
		t.Fatalf("ResolveSong() error = %v", err)
	}
	if link.Provider != "youtube" || !strings.Contains(link.URL, "youtube.com/results?search_query=") {
		t.Fatalf("ResolveSong() = %#v, want YouTube fallback", link)
	}
}

func TestMusicBrainzVerifierReplacesInvalidStarterTrack(t *testing.T) {
	musicBrainz := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/release-group":
			_, _ = writer.Write([]byte(`{
				"release-groups": [{
					"id": "rg-volume-4",
					"title": "Black Sabbath Vol. 4",
					"first-release-date": "1972-09-25",
					"primary-type": "Album",
					"artist-credit": [{"name": "Black Sabbath"}],
					"tags": [{"name": "heavy metal", "count": 5}]
				}]
			}`))
		case "/release":
			_, _ = writer.Write([]byte(`{
				"releases": [{
					"title": "Black Sabbath Vol. 4",
					"date": "1972-09-25",
					"status": "Official",
					"media": [{
						"tracks": [
							{"title": "Wheels of Confusion"},
							{"title": "Tomorrow's Dream"}
						]
					}]
				}]
			}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer musicBrainz.Close()

	verifier := &MusicBrainzAlbumVerifier{
		baseURL:      musicBrainz.URL,
		client:       musicBrainz.Client(),
		requestDelay: 0,
	}

	candidate, ok, err := verifier.Verify(context.Background(), database.RecommendationCandidateInput{
		Artist:       "Black Sabbath",
		Album:        "Volume 4",
		StarterTrack: "War Pigs",
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !ok {
		t.Fatalf("Verify() ok = false, want true")
	}
	if candidate.Album != "Black Sabbath Vol. 4" {
		t.Fatalf("candidate.Album = %q, want canonical MusicBrainz title", candidate.Album)
	}
	if candidate.StarterTrack != "Wheels of Confusion" {
		t.Fatalf("candidate.StarterTrack = %q, want Wheels of Confusion", candidate.StarterTrack)
	}
}

func TestMusicBrainzReleaseRadarParsesNewAlbums(t *testing.T) {
	tagLookupCount := 0
	musicBrainz := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/release-group":
		case "/release-group/newer-album":
			tagLookupCount++
			if request.URL.Query().Get("inc") != "tags" {
				t.Fatalf("tag lookup inc = %q, want tags", request.URL.Query().Get("inc"))
			}
			_, _ = writer.Write([]byte(`{
				"id": "newer-album",
				"title": "Newer Heavy Record",
				"tags": [
					{"name": "Electronic", "count": 8},
					{"name": "laut.de", "count": 7},
					{"name": "Psychedelic rock", "count": 3}
				]
			}`))
			return
		case "/release-group/older-window-album":
			tagLookupCount++
			_, _ = writer.Write([]byte(`{"id": "older-window-album", "title": "Older Window Record", "tags": []}`))
			return
		default:
			http.NotFound(writer, request)
			return
		}
		query := request.URL.Query().Get("query")
		for _, want := range []string{`artist:"Liked Artist"`, "firstreleasedate:[2026-07-01 TO 2026-08-15]"} {
			if !strings.Contains(query, want) {
				t.Fatalf("query = %q, want %q", query, want)
			}
		}
		if request.URL.Query().Get("inc") != "" {
			t.Fatalf("search inc = %q, want empty", request.URL.Query().Get("inc"))
		}
		_, _ = writer.Write([]byte(`{
			"release-groups": [
				{
					"id": "newer-album",
					"title": "Newer Heavy Record",
					"first-release-date": "2026-08-10",
					"primary-type": "Album",
					"artist-credit": [{"name": "Liked Artist"}]
				},
				{
					"id": "new-single",
					"title": "New Single",
					"first-release-date": "2026-08-14",
					"primary-type": "Single",
					"artist-credit": [{"name": "Liked Artist"}]
				},
				{
					"id": "older-window-album",
					"title": "Older Window Record",
					"first-release-date": "2026-07-05",
					"primary-type": "Album",
					"artist-credit": [{"name": "Liked Artist"}]
				},
				{
					"id": "false-positive-album",
					"title": "False Positive Record",
					"first-release-date": "2026-08-12",
					"primary-type": "Album",
					"artist-credit": [{"name": "Liked Artist Ensemble"}]
				},
				{
					"id": "too-old-album",
					"title": "Too Old Record",
					"first-release-date": "2026-06-01",
					"primary-type": "Album",
					"artist-credit": [{"name": "Liked Artist"}]
				}
			]
		}`))
	}))
	defer musicBrainz.Close()

	radar := &MusicBrainzReleaseRadar{
		baseURL:      musicBrainz.URL,
		client:       musicBrainz.Client(),
		requestDelay: 0,
	}
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	releases, err := radar.NewReleases(context.Background(), []database.ArtistAffinity{
		{Artist: "Liked Artist"},
	}, since, until, 3)
	if err != nil {
		t.Fatalf("NewReleases() error = %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("len(releases) = %d, want 2: %#v", len(releases), releases)
	}
	if releases[0].Artist != "Liked Artist" || releases[0].Album != "Newer Heavy Record" || releases[0].ReleaseDate != "2026-08-10" {
		t.Fatalf("releases[0] = %#v, want newest album first", releases[0])
	}
	if !containsString(releases[0].GenreTags, "electronic") || releases[0].Blurb != "Tagged electronic, psychedelic rock." {
		t.Fatalf("releases[0] tags/blurb = %#v / %q, want electronic blurb", releases[0].GenreTags, releases[0].Blurb)
	}
	if releases[1].Album != "Older Window Record" {
		t.Fatalf("releases[1] = %#v, want older in-window album second", releases[1])
	}
	if tagLookupCount != 2 {
		t.Fatalf("tagLookupCount = %d, want 2", tagLookupCount)
	}
}

func TestReleaseRadarUsesListenBrainzFreshReleases(t *testing.T) {
	var musicBrainzSearchCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/explore/fresh-releases":
			if request.URL.Query().Get("days") != "90" {
				t.Fatalf("days = %q, want 90", request.URL.Query().Get("days"))
			}
			_, _ = writer.Write([]byte(`{
				"payload": {
					"releases": [
						{
							"artist_credit_name": "King Gizzard & the Lizard Wizard",
							"release_name": "Alien Metal",
							"release_date": "2026-08-14",
							"release_group_primary_type": "Album",
							"release_group_mbid": "alien-metal-rg",
							"release_tags": ["electronic"]
						},
						{
							"artist_credit_name": "Protest the Hero",
							"release_name": "Within",
							"release_date": "2026-07-17",
							"release_group_primary_type": "Album",
							"release_tags": []
						},
						{
							"artist_credit_name": "Other Artist",
							"release_name": "Not Yours",
							"release_date": "2026-08-10",
							"release_group_primary_type": "Album",
							"release_tags": ["rock"]
						},
						{
							"artist_credit_name": "King Gizzard & the Lizard Wizard",
							"release_name": "Single Thing",
							"release_date": "2026-08-12",
							"release_group_primary_type": "Single",
							"release_tags": ["electronic"]
						}
					]
				}
			}`))
		case "/release-group":
			musicBrainzSearchCalls++
			http.NotFound(writer, request)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	radar := &MusicBrainzReleaseRadar{
		baseURL:         server.URL,
		listenBrainzURL: server.URL,
		iTunesBaseURL:   "",
		client:          server.Client(),
		requestDelay:    0,
	}
	since := time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	releases, err := radar.NewReleases(context.Background(), []database.ArtistAffinity{
		{Artist: "King Gizzard & The Lizard Wizard", CleanArtist: "king gizzard and the lizard wizard"},
		{Artist: "Protest The Hero", CleanArtist: "protest the hero"},
	}, since, until, 8)
	if err != nil {
		t.Fatalf("NewReleases() error = %v", err)
	}
	if musicBrainzSearchCalls != 0 {
		t.Fatalf("musicBrainzSearchCalls = %d, want 0 when ListenBrainz succeeds", musicBrainzSearchCalls)
	}
	if len(releases) != 2 {
		t.Fatalf("len(releases) = %d, want 2: %#v", len(releases), releases)
	}
	if releases[0].Album != "Alien Metal" || releases[0].Blurb != "Tagged electronic." {
		t.Fatalf("releases[0] = %#v, want Alien Metal electronic blurb", releases[0])
	}
	if releases[1].Album != "Within" {
		t.Fatalf("releases[1] = %#v, want Within", releases[1])
	}
}

func TestReleaseRadarScansFullListenBrainzFeedBeforeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path != "/explore/fresh-releases" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte(`{
			"payload": {
				"releases": [
					{
						"artist_credit_name": "Older Match",
						"release_name": "Older Album",
						"release_date": "2026-08-01",
						"release_group_primary_type": "Album",
						"release_tags": ["rock"]
					},
					{
						"artist_credit_name": "Middle Match",
						"release_name": "Middle Album",
						"release_date": "2026-08-10",
						"release_group_primary_type": "Album",
						"release_tags": ["metal"]
					},
					{
						"artist_credit_name": "Newest Match",
						"release_name": "Newest Album",
						"release_date": "2026-08-28",
						"release_group_primary_type": "Album",
						"release_tags": ["bluegrass"]
					}
				]
			}
		}`))
	}))
	defer server.Close()

	radar := &MusicBrainzReleaseRadar{
		listenBrainzURL: server.URL,
		iTunesBaseURL:   "",
		client:          server.Client(),
		requestDelay:    0,
	}
	since := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	releases, err := radar.NewReleases(context.Background(), []database.ArtistAffinity{
		{Artist: "Older Match", CleanArtist: "older match"},
		{Artist: "Middle Match", CleanArtist: "middle match"},
		{Artist: "Newest Match", CleanArtist: "newest match"},
	}, since, until, 2)
	if err != nil {
		t.Fatalf("NewReleases() error = %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("len(releases) = %d, want 2: %#v", len(releases), releases)
	}
	if releases[0].Album != "Newest Album" || releases[1].Album != "Middle Album" {
		t.Fatalf("releases = %#v, want newest two after scanning full feed", releases)
	}
}

func TestReleaseRadarMatchesCollaborativeArtistCredits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path != "/explore/fresh-releases" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte(`{
			"payload": {
				"releases": [
					{
						"artist_credit_name": "CZARFACE & Frankie Pulitzer",
						"release_name": "Czarface Meets Frankie Pulitzer",
						"release_date": "2026-08-28",
						"release_group_primary_type": "Album",
						"release_tags": ["hip hop"]
					}
				]
			}
		}`))
	}))
	defer server.Close()

	radar := &MusicBrainzReleaseRadar{
		listenBrainzURL: server.URL,
		iTunesBaseURL:   "",
		client:          server.Client(),
		requestDelay:    0,
	}
	since := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	releases, err := radar.NewReleases(context.Background(), []database.ArtistAffinity{
		{Artist: "CZARFACE & MF DOOM", CleanArtist: "czarface and mf doom"},
	}, since, until, 8)
	if err != nil {
		t.Fatalf("NewReleases() error = %v", err)
	}
	if len(releases) != 1 || releases[0].Album != "Czarface Meets Frankie Pulitzer" {
		t.Fatalf("releases = %#v, want collaborative CZARFACE match", releases)
	}
}

func TestReleaseRadarAffinityArtistsExcludeNegativeOnlyHistory(t *testing.T) {
	artists := releaseRadarAffinityArtists([]database.ArtistAffinity{
		{Artist: "Favorite Artist", CleanArtist: "favorite artist", FavoriteTracksCount: 1, CurvedAffinityScore: 1},
		{
			Artist:        "Rated Artist",
			CleanArtist:   "rated artist",
			AvgUserRating: sql.NullFloat64{Float64: 4.0, Valid: true},
		},
		{Artist: "Disliked Artist", CleanArtist: "disliked artist", DislikedTracksCount: 1, CurvedAffinityScore: -5},
		{
			Artist:        "Lukewarm Rated Artist",
			CleanArtist:   "lukewarm rated artist",
			AvgUserRating: sql.NullFloat64{Float64: 3.0, Valid: true},
		},
	})

	if len(artists) != 2 {
		t.Fatalf("len(artists) = %d, want 2: %#v", len(artists), artists)
	}
	if artists[0].Artist != "Favorite Artist" || artists[1].Artist != "Rated Artist" {
		t.Fatalf("artists = %#v, want favorite and high-rated artists only", artists)
	}
}

func TestReleaseRadarDoesNotCrawlMusicBrainzWhenListenBrainzFails(t *testing.T) {
	var musicBrainzSearchCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/explore/fresh-releases":
			http.Error(writer, "temporarily unavailable", http.StatusBadGateway)
		case "/release-group":
			musicBrainzSearchCalls++
			http.NotFound(writer, request)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	radar := &MusicBrainzReleaseRadar{
		baseURL:         server.URL,
		listenBrainzURL: server.URL,
		client:          server.Client(),
		requestDelay:    0,
	}
	since := time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	releases, err := radar.NewReleases(context.Background(), []database.ArtistAffinity{
		{Artist: "King Gizzard & The Lizard Wizard"},
	}, since, until, 8)
	if err == nil {
		t.Fatalf("NewReleases() error = nil, want ListenBrainz error")
	}
	if len(releases) != 0 {
		t.Fatalf("len(releases) = %d, want 0", len(releases))
	}
	if musicBrainzSearchCalls != 0 {
		t.Fatalf("musicBrainzSearchCalls = %d, want 0 when ListenBrainz is configured", musicBrainzSearchCalls)
	}
}

func TestMusicBrainzReleaseRadarFallsBackToITunesGenre(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/release-group":
			_, _ = writer.Write([]byte(`{
				"release-groups": [{
					"id": "sparse-album",
					"title": "Alien Metal",
					"first-release-date": "2026-08-14",
					"primary-type": "Album",
					"artist-credit": [{"name": "King Gizzard & the Lizard Wizard"}]
				}]
			}`))
		case "/release-group/sparse-album":
			_, _ = writer.Write([]byte(`{"id": "sparse-album", "title": "Alien Metal", "tags": []}`))
		case "/search":
			if request.URL.Query().Get("entity") != "album" {
				t.Fatalf("entity = %q, want album", request.URL.Query().Get("entity"))
			}
			_, _ = writer.Write([]byte(`{
				"results": [
					{"artistName": "Wrong Artist", "collectionName": "Alien Metal", "primaryGenreName": "Comedy"},
					{"artistName": "King Gizzard & the Lizard Wizard", "collectionName": "Alien Metal", "primaryGenreName": "Electronic"}
				]
			}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	radar := &MusicBrainzReleaseRadar{
		baseURL:       server.URL,
		iTunesBaseURL: server.URL + "/search",
		client:        server.Client(),
		requestDelay:  0,
	}
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	releases, err := radar.NewReleases(context.Background(), []database.ArtistAffinity{
		{Artist: "King Gizzard & The Lizard Wizard"},
	}, since, until, 1)
	if err != nil {
		t.Fatalf("NewReleases() error = %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("len(releases) = %d, want 1: %#v", len(releases), releases)
	}
	if releases[0].Blurb != "Tagged electronic." || !containsString(releases[0].GenreTags, "electronic") {
		t.Fatalf("release tags/blurb = %#v / %q, want iTunes electronic fallback", releases[0].GenreTags, releases[0].Blurb)
	}
}

func openWebTestDB(t *testing.T) *database.DBClient {
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

	db, err := database.InitDB(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		db.Ctx.Close()
	})
	return db
}
