package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nicksunday/music-context-platform/internal/database"
	mcpserver "github.com/nicksunday/music-context-platform/internal/mcp"
)

type fakeRecommender struct {
	draft RecommendationDraft
}

func (fake fakeRecommender) Recommend(context.Context, RecommendationRequest, ProfileContext) (RecommendationDraft, error) {
	return fake.draft, nil
}

type fakeVerifier struct {
	allowed map[string]bool
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

func TestDiscoverySelectionPromptIncludesInterpretedVibe(t *testing.T) {
	prompt := discoverySelectionUserPrompt(
		RecommendationRequest{Message: "great bass lines and heavy or funky or both", Limit: 3},
		ProfileContext{},
		modelDiscoveryPlan{
			VibeSummary:    "prominent low-end drive with either weight or groove",
			RequiredTraits: []string{"new-to-user"},
			FlexibleTraits: []string{"bass/rhythm", "heavy", "funk/groove"},
			FallbackTags:   []string{"funk metal", "groove metal", "post-punk"},
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

func TestRecommendationEndpointRejectsLocalLibraryArtist(t *testing.T) {
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

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body = %s", response.Code, response.Body.String())
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
