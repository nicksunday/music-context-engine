package web

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nicksunday/music-context-platform/internal/database"
	mcpserver "github.com/nicksunday/music-context-platform/internal/mcp"
	recommendation "github.com/nicksunday/music-context-platform/internal/recommendation"
	"github.com/nicksunday/music-context-platform/internal/utils"
)

const (
	defaultArtistLimit             = 12
	defaultGenreLimit              = 20
	defaultFeedbackLimit           = 25
	defaultBatchLimit              = 6
	defaultSessionListLimit        = 25
	maxAlbumBatchLimit             = 10
	maxSongBatchLimit              = 20
	defaultOllamaTimeout           = 3 * time.Minute
	defaultVerifyTimeout           = 10 * time.Second
	defaultVerifyDelay             = time.Second
	defaultLinkerTimeout           = 10 * time.Second
	defaultRadarTimeout            = 25 * time.Second
	defaultRadarArtists            = 1500
	defaultMusicBrainzRadarArtists = 50
	defaultRadarReleases           = 16
	defaultRadarCacheTTL           = 12 * time.Hour
	defaultRadarErrorTTL           = 5 * time.Minute
	defaultRadarRefreshTimeout     = 3 * time.Minute
	listenBrainzResponseLimit      = 32 << 20
	newReleaseWindowDays           = 90
	listenBrainzBaseURL            = "https://api.listenbrainz.org/1"
	musicBrainzBaseURL             = "https://musicbrainz.org/ws/2"
	iTunesSearchBaseURL            = "https://itunes.apple.com/search"
	webUserAgent                   = "music-context-platform/1.0.0 (https://github.com/nicksunday/music-context-platform)"
)

var errReleaseRadarRefreshInProgress = errors.New("release radar refresh is in progress")

//go:embed static/*
var staticFiles embed.FS

type Options struct {
	Recommender              Recommender
	Verifier                 CandidateVerifier
	ReleaseRadar             ReleaseRadarProvider
	LinkResolver             StreamingLinkResolver
	SongLinkResolver         SongLinkResolver
	Model                    string
	OllamaURL                string
	Timeout                  time.Duration
	AppleMusic               AppleMusicPlaylistProvider
	AppleMusicDeveloperToken string
}

type Server struct {
	db                       *sql.DB
	recommender              Recommender
	verifier                 CandidateVerifier
	releaseRadar             ReleaseRadarProvider
	linkResolver             StreamingLinkResolver
	songLinkResolver         SongLinkResolver
	appleMusic               AppleMusicPlaylistProvider
	appleMusicDeveloperToken string
	model                    string
	ollamaURL                string
}

type Recommender interface {
	Recommend(context.Context, RecommendationRequest, ProfileContext) (RecommendationDraft, error)
}

type DiscoveryProvider interface {
	Discover(context.Context, DiscoveryRequest) ([]mcpserver.DiscoveryCandidate, error)
}

type CandidateVerifier interface {
	Verify(context.Context, database.RecommendationCandidateInput) (database.RecommendationCandidateInput, bool, error)
}

type StreamingLinkResolver interface {
	Resolve(context.Context, database.RecommendationCandidateInput) (string, error)
}

type SongLink struct {
	URL      string `json:"streaming_url,omitempty"`
	Provider string `json:"streaming_provider,omitempty"`
	AppURL   string `json:"streaming_app_url,omitempty"`
	TrackID  string `json:"-"`
}

type AppleMusicPlaylistProvider interface {
	ResolveTrack(context.Context, database.RecommendationCandidateInput) (AppleMusicTrack, bool, error)
	CreatePlaylist(context.Context, string, []AppleMusicTrack) (AppleMusicPlaylistResult, error)
}

type AppleMusicTrack struct {
	CandidateID string
	Artist      string
	Song        string
	URL         string
	ID          string
}

type AppleMusicPlaylistResult struct {
	URL       string `json:"url,omitempty"`
	AppURL    string `json:"app_url,omitempty"`
	Reference string `json:"reference,omitempty"`
	Added     int    `json:"added"`
}

type AppleMusicAPIPlaylistProvider struct {
	baseURL        string
	developerToken string
	userToken      string
	storefront     string
	client         *http.Client
}

func NewAppleMusicPlaylistProvider() *AppleMusicAPIPlaylistProvider {
	return &AppleMusicAPIPlaylistProvider{
		baseURL:        "https://api.music.apple.com/v1",
		developerToken: strings.TrimSpace(os.Getenv("APPLE_MUSIC_DEVELOPER_TOKEN")),
		userToken:      strings.TrimSpace(os.Getenv("APPLE_MUSIC_USER_TOKEN")),
		storefront:     strings.TrimSpace(os.Getenv("APPLE_MUSIC_STOREFRONT")),
		client:         &http.Client{Timeout: defaultLinkerTimeout},
	}
}

func (provider *AppleMusicAPIPlaylistProvider) authorized() bool {
	return provider != nil && provider.client != nil && provider.developerToken != "" && provider.userToken != ""
}

func (provider *AppleMusicAPIPlaylistProvider) ResolveTrack(ctx context.Context, candidate database.RecommendationCandidateInput) (AppleMusicTrack, bool, error) {
	if !provider.authorized() {
		return AppleMusicTrack{}, false, errors.New("Apple Music authorization is not configured")
	}
	song := strings.TrimSpace(candidate.Song)
	if song == "" {
		song = strings.TrimSpace(candidate.StarterTrack)
	}
	if strings.TrimSpace(candidate.Artist) == "" || song == "" {
		return AppleMusicTrack{}, false, nil
	}
	lookup := NewAppleMusicLinker()
	link, err := lookup.ResolveSong(ctx, candidate)
	if err != nil || link.TrackID == "" {
		return AppleMusicTrack{}, false, err
	}
	return AppleMusicTrack{Artist: candidate.Artist, Song: song, URL: link.URL, ID: link.TrackID}, true, nil
}

func (provider *AppleMusicAPIPlaylistProvider) CreatePlaylist(ctx context.Context, name string, tracks []AppleMusicTrack) (AppleMusicPlaylistResult, error) {
	if !provider.authorized() {
		return AppleMusicPlaylistResult{}, errors.New("Apple Music authorization is not configured")
	}
	if len(tracks) == 0 {
		return AppleMusicPlaylistResult{}, errors.New("no Apple Music tracks supplied")
	}
	storefront := provider.storefront
	if storefront == "" {
		storefront = "us"
	}
	endpoint := strings.TrimRight(provider.baseURL, "/") + "/users/me/library/playlists"
	payload := struct {
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
	}{}
	payload.Attributes.Name = strings.TrimSpace(name)
	body, err := json.Marshal(payload)
	if err != nil {
		return AppleMusicPlaylistResult{}, err
	}
	playlist, err := provider.doJSON(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return AppleMusicPlaylistResult{}, err
	}
	var created struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				URL string `json:"url"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(playlist, &created); err != nil || len(created.Data) == 0 || created.Data[0].ID == "" {
		return AppleMusicPlaylistResult{}, errors.New("Apple Music returned no playlist reference")
	}
	id := created.Data[0].ID
	items := make([]map[string]string, 0, len(tracks))
	for _, track := range tracks {
		items = append(items, map[string]string{"id": track.ID, "type": "songs"})
	}
	relationshipBody, _ := json.Marshal(map[string]any{"data": items})
	addURL := strings.TrimRight(provider.baseURL, "/") + "/users/me/library/playlists/" + url.PathEscape(id) + "/tracks"
	if _, err := provider.doJSON(ctx, http.MethodPost, addURL, relationshipBody); err != nil {
		return AppleMusicPlaylistResult{Reference: id, URL: strings.TrimSpace(created.Data[0].Attributes.URL), Added: 0}, err
	}
	webURL := strings.TrimSpace(created.Data[0].Attributes.URL)
	if webURL == "" {
		webURL = "https://music.apple.com/" + storefront + "/playlist/" + url.PathEscape(id)
	}
	return AppleMusicPlaylistResult{Reference: id, URL: webURL, AppURL: AppleMusicAppURL(webURL), Added: len(tracks)}, nil
}

func (provider *AppleMusicAPIPlaylistProvider) doJSON(ctx context.Context, method, endpoint string, body []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+provider.developerToken)
	request.Header.Set("Music-User-Token", provider.userToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := provider.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Apple Music returned %s", response.Status)
	}
	return responseBody, nil
}

type PlaylistRequest struct {
	BatchID string `json:"batch_id"`
	Name    string `json:"name,omitempty"`
}

type PlaylistSkipped struct {
	CandidateID string `json:"candidate_id"`
	Artist      string `json:"artist"`
	Song        string `json:"song,omitempty"`
	Reason      string `json:"reason"`
}

type PlaylistResponse struct {
	Playlist AppleMusicPlaylistResult `json:"playlist,omitempty"`
	Added    int                      `json:"added"`
	Skipped  []PlaylistSkipped        `json:"skipped,omitempty"`
	Partial  bool                     `json:"partial"`
}

type AppleMusicConfigResponse struct {
	Enabled        bool   `json:"enabled"`
	DeveloperToken string `json:"developer_token,omitempty"`
}

type SongLinkResolver interface {
	ResolveSong(context.Context, database.RecommendationCandidateInput) (SongLink, error)
}

type ReleaseRadarProvider interface {
	NewReleases(context.Context, []database.ArtistAffinity, time.Time, time.Time, int) ([]NewRelease, error)
}

type DiscoveryRequest struct {
	TargetVibe   string
	FallbackTags []string
	SeedArtists  []string
	Limit        int
	Mode         string
}

type RecommendationRequest struct {
	Message  string                     `json:"message"`
	Mood     string                     `json:"mood,omitempty"`
	Avoid    string                     `json:"avoid,omitempty"`
	Limit    int                        `json:"limit,omitempty"`
	Mode     string                     `json:"mode,omitempty"`
	Examples []recommendation.Reference `json:"examples,omitempty"`
}

type RecommendationDraft struct {
	Reply           string                                  `json:"reply"`
	Candidates      []database.RecommendationCandidateInput `json:"candidates"`
	Diagnostics     RecommendationDiagnostics               `json:"-"`
	References      []recommendation.Reference              `json:"references,omitempty"`
	SimilarityEdges []recommendation.SimilarityEdge         `json:"similarity_edges,omitempty"`
	Providers       []recommendation.ProviderStatus         `json:"providers,omitempty"`
	DegradedReason  string                                  `json:"degraded_reason,omitempty"`
	ShortfallReason string                                  `json:"shortfall_reason,omitempty"`
}

type RecommendationDiagnostics struct {
	RequestedCount         int
	DiscoveryPoolCount     int
	DiscoveryAvoidedCount  int
	ModelSelectedCount     int
	BackfillAddedCount     int
	VerifiedCount          int
	VerificationRejected   int
	QualificationRejected  int
	ExcludedCount          int
	AvoidedCount           int
	DiversityRejectedCount int
	ReturnedCount          int
	Stages                 []recommendation.StageMetric `json:"stages,omitempty"`
}

func appendStageMetric(metrics []recommendation.StageMetric, stage string, started time.Time, externalCalls int) []recommendation.StageMetric {
	return append(metrics, recommendation.StageMetric{Stage: stage, ElapsedMS: time.Since(started).Milliseconds(), ExternalCalls: externalCalls})
}

// OfflineCandidate is the bounded candidate shape used by deterministic
// evaluation. It intentionally contains no database or network fields.
type OfflineCandidate struct {
	ID        string
	Artist    string
	Album     string
	Song      string
	GenreTags []string
	Rank      int
	Eligible  bool
}

type OfflinePolicyRequest struct {
	Message string
	Mood    string
	Avoid   string
	Mode    string
}

// ReplayOfflineCandidatePolicy reuses production prompt scoring, avoidance,
// and diversity rules while replacing interactive randomness with a seed.
func ReplayOfflineCandidatePolicy(request OfflinePolicyRequest, candidates []OfflineCandidate, seed int64) []OfflineCandidate {
	webRequest := RecommendationRequest{Message: request.Message, Mood: request.Mood, Avoid: request.Avoid, Mode: recommendationMode(request.Mode)}
	plan := calibrateDiscoveryPlanForPrompt(webRequest, modelDiscoveryPlan{})
	discovery := make([]mcpserver.DiscoveryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.Eligible {
			continue
		}
		discovery = append(discovery, mcpserver.DiscoveryCandidate{ID: candidate.ID, Artist: candidate.Artist, Album: candidate.Album, TrackName: candidate.Song, GenreTags: append([]string(nil), candidate.GenreTags...), ReleaseYear: 0})
	}
	discovery = filterAvoidedDiscoveryCandidates(discovery, requestAvoidTags(webRequest))
	discovery = rankDiscoveryCandidatesForPromptSeeded(webRequest, plan, discovery, seed)
	discovery = mcpserver.DiversifyDiscoveryCandidatesForEvaluation(discovery, plan.FallbackTags, seed)
	result := make([]OfflineCandidate, 0, len(discovery))
	for index, candidate := range discovery {
		result = append(result, OfflineCandidate{ID: candidate.ID, Artist: candidate.Artist, Album: candidate.Album, Song: candidate.TrackName, GenreTags: candidate.GenreTags, Rank: index + 1, Eligible: true})
	}
	return result
}

func (diagnostics RecommendationDiagnostics) shortfallReason() string {
	if diagnostics.ReturnedCount >= diagnostics.RequestedCount {
		return ""
	}
	if diagnostics.DiversityRejectedCount > 0 {
		return "diversity_filtered"
	}
	if diagnostics.VerificationRejected > 0 && diagnostics.VerifiedCount < diagnostics.RequestedCount {
		return "verification_rejected"
	}
	if diagnostics.DiscoveryPoolCount < diagnostics.RequestedCount {
		return "discovery_pool_exhausted"
	}
	if diagnostics.ExcludedCount > 0 || diagnostics.AvoidedCount > 0 || diagnostics.DiscoveryAvoidedCount > 0 {
		return "excluded_or_avoided"
	}
	return "eligible_pool_exhausted"
}

func songRecommendationMode(request RecommendationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(request.Mode), "song") || strings.EqualFold(strings.TrimSpace(request.Mode), "songs")
}

func recommendationMode(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "song") || strings.EqualFold(strings.TrimSpace(value), "songs") {
		return "song"
	}
	return "album"
}

type ProfileContext struct {
	Artists         []database.ArtistAffinity              `json:"artists"`
	Genres          []database.GenreTopography             `json:"genres"`
	RecentFeedback  []database.RecommendationFeedbackLog   `json:"recent_feedback"`
	RelevantContext []database.RecommendationContextRecord `json:"relevant_context,omitempty"`
}

type ContextResponse struct {
	Model          string                               `json:"model"`
	OllamaURL      string                               `json:"ollama_url"`
	Artists        []database.ArtistAffinity            `json:"artists"`
	Genres         []database.GenreTopography           `json:"genres"`
	RecentFeedback []database.RecommendationFeedbackLog `json:"recent_feedback"`
	Verdicts       []string                             `json:"verdicts"`
}

type RecommendationResponse struct {
	Reply           string                       `json:"reply"`
	Batch           database.RecommendationBatch `json:"batch"`
	DegradedReason  string                       `json:"degraded_reason,omitempty"`
	ShortfallReason string                       `json:"shortfall_reason,omitempty"`
}

type BatchResponse struct {
	Batch *database.RecommendationBatch `json:"batch"`
}

type BatchesResponse struct {
	Batches []database.RecommendationBatchSummary `json:"batches"`
}

type RailResponse struct {
	NewReleases  []NewRelease `json:"new_releases"`
	ReleaseError string       `json:"release_error,omitempty"`
}

type NewRelease struct {
	Artist      string   `json:"artist"`
	Album       string   `json:"album"`
	ReleaseDate string   `json:"release_date,omitempty"`
	ReleaseType string   `json:"release_type,omitempty"`
	GenreTags   []string `json:"genre_tags,omitempty"`
	Blurb       string   `json:"blurb,omitempty"`
}

func NewServer(db *sql.DB, options Options) http.Handler {
	server := &Server{
		db:                       db,
		recommender:              options.Recommender,
		verifier:                 options.Verifier,
		releaseRadar:             options.ReleaseRadar,
		linkResolver:             options.LinkResolver,
		songLinkResolver:         options.SongLinkResolver,
		appleMusic:               options.AppleMusic,
		appleMusicDeveloperToken: strings.TrimSpace(options.AppleMusicDeveloperToken),
		model:                    strings.TrimSpace(options.Model),
		ollamaURL:                normalizeBaseURL(options.OllamaURL),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/context", server.handleContext)
	mux.HandleFunc("GET /api/rail", server.handleRail)
	mux.HandleFunc("GET /api/batch/latest", server.handleLatestBatch)
	mux.HandleFunc("GET /api/batch", server.handleBatchByID)
	mux.HandleFunc("GET /api/trace", server.handleTrace)
	mux.HandleFunc("GET /api/batches", server.handleBatches)
	mux.HandleFunc("POST /api/recommendations", server.handleRecommendations)
	mux.HandleFunc("GET /api/examples", server.handleExamples)
	mux.HandleFunc("POST /api/examples", server.handleExamples)
	mux.HandleFunc("DELETE /api/examples", server.handleExamples)
	mux.HandleFunc("POST /api/feedback", server.handleFeedback)
	mux.HandleFunc("POST /api/prompt-fit", server.handlePromptFitFeedback)
	mux.HandleFunc("DELETE /api/prompt-fit", server.handlePromptFitFeedback)
	mux.HandleFunc("GET /api/export/example", server.handleExportExample)
	mux.HandleFunc("GET /api/export/review", server.handleExportReview)
	mux.HandleFunc("POST /api/export/draft", server.handleExportDraft)
	mux.HandleFunc("POST /api/export/approve", server.handleExportApprove)
	mux.HandleFunc("POST /api/export/download", server.handleExportDownload)
	mux.HandleFunc("POST /api/playlists/apple-music", server.handleAppleMusicPlaylist)
	mux.HandleFunc("GET /api/apple-music/config", server.handleAppleMusicConfig)
	mux.Handle("/", staticHandler())
	return mux
}

func (server *Server) handleAppleMusicConfig(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, AppleMusicConfigResponse{
		Enabled:        strings.TrimSpace(server.appleMusicDeveloperToken) != "",
		DeveloperToken: strings.TrimSpace(server.appleMusicDeveloperToken),
	})
}

func NewOllamaRecommender(baseURL, model string) *OllamaRecommender {
	return NewOllamaRecommenderWithTimeout(baseURL, model, defaultOllamaTimeout)
}

func NewOllamaRecommenderWithTimeout(baseURL, model string, timeout time.Duration) *OllamaRecommender {
	if timeout <= 0 {
		timeout = defaultOllamaTimeout
	}
	return &OllamaRecommender{
		baseURL: normalizeBaseURL(baseURL),
		model:   strings.TrimSpace(model),
		client:  &http.Client{Timeout: timeout},
	}
}

func NewMCPDiscoveryProvider(db *sql.DB) *MCPDiscoveryProvider {
	return &MCPDiscoveryProvider{db: db}
}

func NewMCPGroundedOllamaRecommender(db *sql.DB, baseURL, model string, timeout time.Duration) *MCPGroundedOllamaRecommender {
	return newMCPGroundedOllamaRecommenderWithDiscovery(
		baseURL,
		model,
		timeout,
		NewMCPDiscoveryProvider(db),
	)
}

func newMCPGroundedOllamaRecommenderWithDiscovery(
	baseURL string,
	model string,
	timeout time.Duration,
	discovery DiscoveryProvider,
) *MCPGroundedOllamaRecommender {
	return &MCPGroundedOllamaRecommender{
		ollama:    NewOllamaRecommenderWithTimeout(baseURL, model, timeout),
		discovery: discovery,
		db:        discoveryDatabase(discovery),
	}
}

func discoveryDatabase(provider DiscoveryProvider) *sql.DB {
	if provider, ok := provider.(*MCPDiscoveryProvider); ok && provider != nil {
		return provider.db
	}
	return nil
}

func staticHandler() http.Handler {
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(staticRoot))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" || strings.HasPrefix(request.URL.Path, "/recommendations") {
			http.ServeFileFS(writer, request, staticRoot, "index.html")
			return
		}
		fileServer.ServeHTTP(writer, request)
	})
}

func (server *Server) handleContext(writer http.ResponseWriter, request *http.Request) {
	profile, err := server.fetchProfileContext(request.Context())
	if err != nil {
		writeJSONError(writer, http.StatusInternalServerError, "Unable to load music profile context.")
		return
	}

	writeJSON(writer, http.StatusOK, ContextResponse{
		Model:          server.model,
		OllamaURL:      server.ollamaURL,
		Artists:        profile.Artists,
		Genres:         profile.Genres,
		RecentFeedback: profile.RecentFeedback,
		Verdicts:       []string{"disliked", "not_for_me_today", "ok", "good", "great", "already_know"},
	})
}

func (server *Server) handleRail(writer http.ResponseWriter, request *http.Request) {
	artists, err := database.FetchTopArtistAffinities(request.Context(), server.db, defaultRadarArtists)
	if err != nil {
		writeJSONError(writer, http.StatusInternalServerError, "Unable to load artist context for the sidebar.")
		return
	}
	artists = releaseRadarAffinityArtists(artists)

	response := RailResponse{}
	if server.releaseRadar != nil {
		until := dayStart(time.Now())
		since := until.AddDate(0, 0, -newReleaseWindowDays)
		releases, err := server.releaseRadar.NewReleases(request.Context(), artists, since, until, defaultRadarReleases)
		response.NewReleases = releases
		if err != nil {
			if errors.Is(err, errReleaseRadarRefreshInProgress) {
				response.ReleaseError = "Scanning new releases from liked artists. Refresh again shortly."
			} else if len(releases) > 0 {
				response.ReleaseError = "Some new release lookups failed; showing partial results."
			} else {
				response.ReleaseError = "Unable to load new releases from the release feed right now."
			}
		}
	}

	writeJSON(writer, http.StatusOK, response)
}

func (server *Server) handleRecommendations(writer http.ResponseWriter, request *http.Request) {
	if server.recommender == nil {
		writeJSONError(writer, http.StatusServiceUnavailable, "Recommendation model is not configured.")
		return
	}

	var input RecommendationRequest
	if err := decodeJSON(request, &input); err != nil {
		writeJSONError(writer, http.StatusBadRequest, "Request body must be a JSON object.")
		return
	}
	input.Message = strings.TrimSpace(input.Message)
	input.Mood = strings.TrimSpace(input.Mood)
	input.Avoid = strings.TrimSpace(input.Avoid)
	if err := validateRecommendationExamples(input.Examples); err != nil {
		writeJSONError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if songRecommendationMode(input) {
		input.Mode = "song"
	} else {
		input.Mode = "album"
	}
	input.Limit = clampBatchLimit(input.Mode, input.Limit)
	if input.Message == "" {
		writeJSONError(writer, http.StatusBadRequest, "Please provide a recommendation prompt.")
		return
	}

	profile, err := server.fetchProfileContext(request.Context())
	if err != nil {
		writeJSONError(writer, http.StatusInternalServerError, "Unable to load music profile context.")
		return
	}
	contextTerms := recommendationContextTerms(input)
	if names, nameErr := database.FindExactContextNames(request.Context(), server.db, strings.Join([]string{input.Message, input.Mood, input.Avoid}, " "), 24); nameErr == nil {
		contextTerms = append(contextTerms, names...)
	}
	if relevant, contextErr := database.FetchRelevantRecommendationContext(request.Context(), server.db, contextTerms, 24); contextErr == nil {
		profile.RelevantContext = relevant
	}
	exclusions, err := (&database.DB{Ctx: server.db}).GetDiscoveryAlbumExclusionsContext(request.Context())
	if err != nil {
		writeJSONError(writer, http.StatusInternalServerError, "Unable to load local discovery exclusions.")
		return
	}
	avoidTags := requestAvoidTags(input)

	draft, err := server.recommender.Recommend(request.Context(), input, profile)
	if err != nil {
		writeJSONError(writer, http.StatusBadGateway, err.Error())
		return
	}
	if len(draft.Candidates) == 0 {
		writeJSONError(writer, http.StatusBadGateway, recommendationCandidateEmptyMessage(input))
		return
	}
	draft.Diagnostics.RequestedCount = input.Limit
	before := len(draft.Candidates)
	draft.Candidates = filterExcludedCandidates(draft.Candidates, exclusions)
	draft.Diagnostics.ExcludedCount += before - len(draft.Candidates)
	before = len(draft.Candidates)
	draft.Candidates = filterAvoidedRecommendationCandidates(draft.Candidates, avoidTags)
	draft.Diagnostics.AvoidedCount += before - len(draft.Candidates)
	if songRecommendationMode(input) {
		for idx := range draft.Candidates {
			if strings.TrimSpace(draft.Candidates[idx].Song) == "" {
				draft.Candidates[idx].Song = strings.TrimSpace(draft.Candidates[idx].StarterTrack)
			}
		}
	}
	if len(draft.Candidates) == 0 {
		writeJSONError(writer, http.StatusBadGateway, "The recommendation model only returned albums blocked by ratings, recommendation feedback, or avoid filters. Try again with a more specific prompt.")
		return
	}
	verificationStarted := time.Now()
	before = len(draft.Candidates)
	draft.Candidates, err = server.verifyCandidates(request.Context(), draft.Candidates)
	draft.Diagnostics.Stages = appendStageMetric(draft.Diagnostics.Stages, "verification", verificationStarted, before)
	draft.Diagnostics.VerifiedCount = len(draft.Candidates)
	draft.Diagnostics.VerificationRejected += before - len(draft.Candidates)
	if err != nil {
		writeJSONError(writer, http.StatusBadGateway, err.Error())
		return
	}
	before = len(draft.Candidates)
	draft.Candidates = filterExcludedCandidates(draft.Candidates, exclusions)
	draft.Diagnostics.ExcludedCount += before - len(draft.Candidates)
	before = len(draft.Candidates)
	draft.Candidates = filterAvoidedRecommendationCandidates(draft.Candidates, avoidTags)
	draft.Diagnostics.AvoidedCount += before - len(draft.Candidates)
	if len(draft.Candidates) == 0 {
		writeJSONError(writer, http.StatusBadGateway, "No generated album candidates survived external verification, album-level exclusions, and avoid filters. Try again with a more specific prompt.")
		return
	}
	before = len(draft.Candidates)
	draft.Candidates = applyRecommendationBatchDiversity(input, draft.Candidates)
	draft.Diagnostics.DiversityRejectedCount += before - len(draft.Candidates)
	if len(draft.Candidates) == 0 {
		writeJSONError(writer, http.StatusBadGateway, "No generated album candidates survived final artist-diversity filtering. Try again with a broader prompt.")
		return
	}
	if len(draft.Candidates) > input.Limit {
		draft.Candidates = draft.Candidates[:input.Limit]
		resetRecommendationCandidateRanks(draft.Candidates)
	}
	draft.Diagnostics.ReturnedCount = len(draft.Candidates)
	if reason := draft.Diagnostics.shortfallReason(); reason != "" {
		draft.ShortfallReason = reason
		if draft.Reply == "" {
			draft.Reply = "Returned a smaller batch because there were not enough qualified candidates (" + reason + ")."
		}
		log.Printf("recommendation_shortfall mode=%s requested=%d returned=%d discovery_pool=%d discovery_avoided=%d model_selected=%d backfill_added=%d verified=%d verification_rejected=%d excluded=%d avoided=%d diversity_rejected=%d reason=%s", input.Mode, draft.Diagnostics.RequestedCount, draft.Diagnostics.ReturnedCount, draft.Diagnostics.DiscoveryPoolCount, draft.Diagnostics.DiscoveryAvoidedCount, draft.Diagnostics.ModelSelectedCount, draft.Diagnostics.BackfillAddedCount, draft.Diagnostics.VerifiedCount, draft.Diagnostics.VerificationRejected, draft.Diagnostics.ExcludedCount, draft.Diagnostics.AvoidedCount, draft.Diagnostics.DiversityRejectedCount, reason)
	}
	destinationStarted := time.Now()
	if songRecommendationMode(input) {
		draft.Candidates = server.resolveSongLinks(request.Context(), draft.Candidates)
	} else {
		draft.Candidates = server.resolveCandidateLinks(request.Context(), draft.Candidates)
	}
	draft.Diagnostics.Stages = appendStageMetric(draft.Diagnostics.Stages, "destination_resolution", destinationStarted, len(draft.Candidates))
	draft.References = append([]recommendation.Reference(nil), draft.References...)

	persistenceStarted := time.Now()
	batch, err := database.CreateRecommendationBatch(request.Context(), server.db, database.RecommendationBatchInput{
		Prompt:     input.Message,
		Mood:       input.Mood,
		Notes:      draft.Reply,
		Mode:       input.Mode,
		Candidates: draft.Candidates,
		Snapshot: &database.RecommendationBatchSnapshotInput{
			SchemaVersion: 1,
			Complete:      true,
			Payload: map[string]any{
				"request":          map[string]any{"message": input.Message, "mood": input.Mood, "avoid": input.Avoid, "limit": input.Limit, "mode": input.Mode},
				"examples":         input.Examples,
				"references":       draft.References,
				"providers":        draft.Providers,
				"similarity_edges": draft.SimilarityEdges,
				"model":            server.model,
				"ollama_url":       server.ollamaURL,
				"reply":            strings.TrimSpace(draft.Reply),
				"candidate_output": draft.Candidates,
				"diagnostics":      draft.Diagnostics,
				"degraded_reason":  draft.DegradedReason,
				"shortfall_reason": draft.ShortfallReason,
			},
		},
	})
	draft.Diagnostics.Stages = appendStageMetric(draft.Diagnostics.Stages, "persistence", persistenceStarted, 1)
	if err != nil {
		writeJSONError(writer, http.StatusInternalServerError, "Unable to persist recommendation batch.")
		return
	}

	writeJSON(writer, http.StatusOK, RecommendationResponse{
		Reply:           strings.TrimSpace(draft.Reply),
		Batch:           batch,
		DegradedReason:  draft.DegradedReason,
		ShortfallReason: draft.ShortfallReason,
	})
}

func (server *Server) handleExamples(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Message string                   `json:"message"`
		Mood    string                   `json:"mood,omitempty"`
		Avoid   string                   `json:"avoid,omitempty"`
		Mode    string                   `json:"mode,omitempty"`
		Example recommendation.Reference `json:"example"`
	}
	if request.Method == http.MethodGet {
		input.Message = request.URL.Query().Get("message")
		input.Mood = request.URL.Query().Get("mood")
		input.Avoid = request.URL.Query().Get("avoid")
		input.Mode = request.URL.Query().Get("mode")
	} else if err := decodeJSON(request, &input); err != nil {
		writeJSONError(writer, http.StatusBadRequest, "Request body must be a JSON object.")
		return
	}
	mode := recommendationMode(input.Mode)
	requestKey := database.RecommendationRequestKey(input.Message, input.Mood, input.Avoid, mode)
	if request.Method == http.MethodGet {
		examples, err := database.ListRecommendationExamples(request.Context(), server.db, requestKey)
		if err != nil {
			writeJSONError(writer, http.StatusInternalServerError, "Unable to load recommendation examples.")
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"request_key": requestKey, "examples": examples})
		return
	}
	if err := validateRecommendationExamples([]recommendation.Reference{input.Example}); err != nil {
		writeJSONError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if request.Method == http.MethodDelete {
		err := database.ClearRecommendationExample(request.Context(), server.db, requestKey, input.Example.EntityScope, input.Example.SuppliedText, input.Example.Artist, input.Example.Album, input.Example.Song)
		if err != nil {
			writeJSONError(writer, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"cleared": true, "request_key": requestKey})
		return
	}
	example, err := database.SaveRecommendationExample(request.Context(), server.db, database.RecommendationExample{
		RequestKey: requestKey, EntityScope: input.Example.EntityScope, SuppliedText: input.Example.SuppliedText,
		Artist: input.Example.Artist, Album: input.Example.Album, Song: input.Example.Song,
		Polarity: input.Example.Polarity, Notes: input.Example.Notes,
	})
	if err != nil {
		writeJSONError(writer, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, example)
}

func recommendationCandidateEmptyMessage(request RecommendationRequest) string {
	if songRecommendationMode(request) {
		return "The recommendation model returned no song candidates."
	}
	return "The recommendation model returned no album candidates."
}

func (server *Server) handleFeedback(writer http.ResponseWriter, request *http.Request) {
	var input database.RecommendationFeedbackInput
	if err := decodeJSON(request, &input); err != nil {
		writeJSONError(writer, http.StatusBadRequest, "Request body must be a JSON object.")
		return
	}
	input.Artist = strings.TrimSpace(input.Artist)
	input.Album = strings.TrimSpace(input.Album)
	input.Verdict = strings.TrimSpace(input.Verdict)
	if input.Artist == "" || input.Album == "" || input.Verdict == "" {
		writeJSONError(writer, http.StatusBadRequest, "Feedback requires artist, album, and verdict.")
		return
	}

	result, err := database.LogRecommendationFeedback(request.Context(), server.db, input)
	if err != nil {
		writeJSONError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if result.CleanArtist == "" || result.CleanTitle == "" {
		writeJSONError(writer, http.StatusBadRequest, "Artist and album must normalize to non-empty lookup tokens.")
		return
	}

	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handlePromptFitFeedback(writer http.ResponseWriter, request *http.Request) {
	var input database.RecommendationPromptFitFeedbackInput
	if err := decodeJSON(request, &input); err != nil {
		writeJSONError(writer, http.StatusBadRequest, "Request body must be a JSON object.")
		return
	}
	input.BatchID = strings.TrimSpace(input.BatchID)
	input.CandidateID = strings.TrimSpace(input.CandidateID)
	if request.Method == http.MethodDelete {
		if err := database.ClearRecommendationPromptFitFeedback(request.Context(), server.db, input.BatchID, input.CandidateID); err != nil {
			writeJSONError(writer, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"cleared": true, "batch_id": input.BatchID, "candidate_id": input.CandidateID})
		return
	}
	result, err := database.SaveRecommendationPromptFitFeedback(request.Context(), server.db, input)
	if err != nil {
		writeJSONError(writer, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) handleExportExample(writer http.ResponseWriter, request *http.Request) {
	example, err := database.BuildRecommendationExportExample(request.Context(), server.db, request.URL.Query().Get("feedback_id"))
	if err != nil {
		writeJSONError(writer, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, example)
}

func (server *Server) handleExportReview(writer http.ResponseWriter, request *http.Request) {
	items, err := database.ListRecommendationExportReview(request.Context(), server.db, 100)
	if err != nil {
		writeJSONError(writer, http.StatusInternalServerError, "Unable to load export review items.")
		return
	}
	if items == nil {
		items = []database.RecommendationExportReviewItem{}
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": items})
}

func (server *Server) handleExportDraft(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		FeedbackID string          `json:"feedback_id"`
		Payload    json.RawMessage `json:"payload"`
	}
	if err := decodeJSON(request, &input); err != nil || len(input.Payload) == 0 {
		writeJSONError(writer, http.StatusBadRequest, "feedback_id and payload are required")
		return
	}
	draft, err := database.SaveRecommendationExportDraft(request.Context(), server.db, input.FeedbackID, string(input.Payload))
	if err != nil {
		writeJSONError(writer, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, draft)
}

func (server *Server) handleExportApprove(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		DraftID string `json:"draft_id"`
	}
	if err := decodeJSON(request, &input); err != nil || strings.TrimSpace(input.DraftID) == "" {
		writeJSONError(writer, http.StatusBadRequest, "draft_id is required")
		return
	}
	if err := database.ApproveRecommendationExportDraft(request.Context(), server.db, input.DraftID); err != nil {
		writeJSONError(writer, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"approved": true, "draft_id": input.DraftID})
}

func (server *Server) handleExportDownload(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		DraftIDs []string `json:"draft_ids"`
	}
	if err := decodeJSON(request, &input); err != nil || len(input.DraftIDs) == 0 {
		writeJSONError(writer, http.StatusBadRequest, "draft_ids are required")
		return
	}
	sort.Strings(input.DraftIDs)
	drafts, err := database.FetchApprovedRecommendationExportDrafts(request.Context(), server.db, input.DraftIDs)
	if err != nil {
		writeJSONError(writer, http.StatusBadRequest, err.Error())
		return
	}
	writer.Header().Set("Content-Type", "application/x-ndjson")
	writer.Header().Set("Content-Disposition", "attachment; filename=music-vault-fit-examples.jsonl")
	for _, draft := range drafts {
		if _, err := io.WriteString(writer, draft.Payload+"\n"); err != nil {
			return
		}
	}
}

func (server *Server) handleAppleMusicPlaylist(writer http.ResponseWriter, request *http.Request) {
	if server.appleMusic == nil {
		writeJSONError(writer, http.StatusServiceUnavailable, "Apple Music playlist authorization is not configured. Set the local Apple Music credentials first.")
		return
	}
	var input PlaylistRequest
	if err := decodeJSON(request, &input); err != nil {
		writeJSONError(writer, http.StatusBadRequest, "Request body must be a JSON object.")
		return
	}
	batch, err := database.FetchRecommendationBatchByID(request.Context(), server.db, input.BatchID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(writer, http.StatusNotFound, "Recommendation batch not found.")
		return
	}
	if err != nil {
		writeJSONError(writer, http.StatusInternalServerError, "Unable to load the recommendation batch.")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(batch.Mode), "song") {
		writeJSONError(writer, http.StatusBadRequest, "Apple Music playlists are available only for Individual Song batches.")
		return
	}
	if len(batch.Candidates) == 0 {
		writeJSONError(writer, http.StatusBadRequest, "This recommendation batch has no songs to add.")
		return
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Music Vault — " + strings.TrimSpace(batch.Prompt)
	}
	tracks := make([]AppleMusicTrack, 0, len(batch.Candidates))
	skipped := make([]PlaylistSkipped, 0)
	seen := make(map[string]bool)
	for _, candidate := range batch.Candidates {
		track, ok, resolveErr := server.appleMusic.ResolveTrack(request.Context(), database.RecommendationCandidateInput{
			Artist: candidate.Artist, Album: candidate.Album, Song: candidate.Song, StarterTrack: candidate.StarterTrack,
		})
		if resolveErr != nil || !ok || strings.TrimSpace(track.ID) == "" {
			skipped = append(skipped, PlaylistSkipped{CandidateID: candidate.ID, Artist: candidate.Artist, Song: candidate.Song, Reason: "Could not resolve an Apple Music track"})
			continue
		}
		if seen[track.ID] {
			skipped = append(skipped, PlaylistSkipped{CandidateID: candidate.ID, Artist: candidate.Artist, Song: candidate.Song, Reason: "Duplicate Apple Music track"})
			continue
		}
		seen[track.ID] = true
		track.CandidateID = candidate.ID
		tracks = append(tracks, track)
	}
	if len(tracks) == 0 {
		writeJSONError(writer, http.StatusBadRequest, "No recommendation songs could be resolved to Apple Music tracks; no playlist was created.")
		return
	}
	playlist, err := server.appleMusic.CreatePlaylist(request.Context(), name, tracks)
	if err != nil {
		writeJSONError(writer, http.StatusBadGateway, "Apple Music playlist creation failed: "+err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, PlaylistResponse{Playlist: playlist, Added: playlist.Added, Skipped: skipped, Partial: len(skipped) > 0})
}

func (server *Server) handleLatestBatch(writer http.ResponseWriter, request *http.Request) {
	mode := recommendationMode(request.URL.Query().Get("mode"))
	batch, err := database.FetchLatestRecommendationBatchForMode(request.Context(), server.db, mode)
	if err != nil {
		writeJSONError(writer, http.StatusInternalServerError, "Unable to load the latest recommendation batch.")
		return
	}
	if batch.ID == "" {
		writeJSON(writer, http.StatusOK, BatchResponse{Batch: nil})
		return
	}
	writeJSON(writer, http.StatusOK, BatchResponse{Batch: &batch})
}

func (server *Server) handleBatchByID(writer http.ResponseWriter, request *http.Request) {
	id := strings.TrimSpace(request.URL.Query().Get("id"))
	if id == "" {
		writeJSONError(writer, http.StatusBadRequest, "Missing id query parameter.")
		return
	}

	batch, err := database.FetchRecommendationBatchByID(request.Context(), server.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(writer, http.StatusNotFound, "Recommendation batch not found.")
			return
		}
		writeJSONError(writer, http.StatusInternalServerError, "Unable to load the recommendation batch.")
		return
	}
	requestedMode := strings.TrimSpace(request.URL.Query().Get("mode"))
	if requestedMode != "" && recommendationMode(batch.Mode) != recommendationMode(requestedMode) {
		writeJSONError(writer, http.StatusNotFound, "Recommendation batch not found for this mode.")
		return
	}
	writeJSON(writer, http.StatusOK, BatchResponse{Batch: &batch})
}

type RecommendationTraceResponse struct {
	BatchID   string               `json:"batch_id"`
	Available bool                 `json:"available"`
	Missing   []string             `json:"missing,omitempty"`
	Trace     recommendation.Trace `json:"trace"`
}

func (server *Server) handleTrace(writer http.ResponseWriter, request *http.Request) {
	id := strings.TrimSpace(request.URL.Query().Get("id"))
	if id == "" {
		writeJSONError(writer, http.StatusBadRequest, "Missing id query parameter.")
		return
	}
	batch, err := database.FetchRecommendationBatchByID(request.Context(), server.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(writer, http.StatusNotFound, "Recommendation batch not found.")
			return
		}
		writeJSONError(writer, http.StatusInternalServerError, "Unable to load recommendation trace.")
		return
	}
	response := RecommendationTraceResponse{BatchID: batch.ID, Trace: recommendation.Trace{SchemaVersion: recommendation.CurrentSchemaVersion}}
	if !batch.SnapshotAvailable || batch.Snapshot == nil {
		response.Trace.Legacy = true
		response.Missing = []string{"snapshot", "references", "examples", "provider_status", "similarity_edges", "candidate_evidence", "qualification", "stage_metrics"}
		writeJSON(writer, http.StatusOK, response)
		return
	}
	response.Available = true
	response.Trace = traceFromSnapshotPayload(batch.Snapshot.Payload)
	writeJSON(writer, http.StatusOK, response)
}

func traceFromSnapshotPayload(payload map[string]any) recommendation.Trace {
	trace := recommendation.Trace{SchemaVersion: recommendation.CurrentSchemaVersion}
	decode := func(key string, target any) {
		raw, ok := payload[key]
		if !ok {
			return
		}
		encoded, err := json.Marshal(raw)
		if err == nil {
			_ = json.Unmarshal(encoded, target)
		}
	}
	decode("references", &trace.References)
	decode("examples", &trace.Examples)
	decode("providers", &trace.Providers)
	decode("similarity_edges", &trace.SimilarityEdges)
	decode("stages", &trace.Stages)
	if diagnostics, ok := payload["diagnostics"].(map[string]any); ok {
		if stages, ok := diagnostics["stages"]; ok {
			decodeValue(stages, &trace.Stages)
		}
	}
	if candidates, ok := payload["candidate_output"].([]any); ok {
		for _, candidate := range candidates {
			if item, ok := candidate.(map[string]any); ok {
				if artist, _ := item["artist"].(string); artist != "" {
					if album, _ := item["album"].(string); album != "" {
						trace.FinalCandidateIDs = append(trace.FinalCandidateIDs, artist+" / "+album)
					}
				}
			}
		}
	}
	if reason, ok := payload["degraded_reason"].(string); ok {
		trace.DegradedReason = reason
	}
	if reason, ok := payload["shortfall_reason"].(string); ok {
		trace.ShortfallReason = reason
	}
	return trace.Normalize()
}

func decodeValue(value any, target any) {
	encoded, err := json.Marshal(value)
	if err == nil {
		_ = json.Unmarshal(encoded, target)
	}
}

func (server *Server) handleBatches(writer http.ResponseWriter, request *http.Request) {
	mode := recommendationMode(request.URL.Query().Get("mode"))
	batches, err := database.ListRecommendationBatchesForMode(request.Context(), server.db, defaultSessionListLimit, mode)
	if err != nil {
		writeJSONError(writer, http.StatusInternalServerError, "Unable to load past recommendation sessions.")
		return
	}
	if batches == nil {
		batches = []database.RecommendationBatchSummary{}
	}
	writeJSON(writer, http.StatusOK, BatchesResponse{Batches: batches})
}

func (server *Server) fetchProfileContext(ctx context.Context) (ProfileContext, error) {
	artists, err := database.FetchTopArtistAffinities(ctx, server.db, defaultArtistLimit)
	if err != nil {
		return ProfileContext{}, err
	}
	genres, err := database.FetchTopGenreTopography(ctx, server.db, defaultGenreLimit)
	if err != nil {
		return ProfileContext{}, err
	}
	feedback, err := database.FetchRecentRecommendationFeedback(ctx, server.db, defaultFeedbackLimit)
	if err != nil {
		return ProfileContext{}, err
	}
	return ProfileContext{
		Artists:        artists,
		Genres:         genres,
		RecentFeedback: feedback,
	}, nil
}

func recommendationContextTerms(request RecommendationRequest) []string {
	terms := []string{request.Message, request.Mood}
	for _, example := range request.Examples {
		terms = append(terms, example.Artist, example.Album, example.Song, example.SuppliedText)
	}
	return terms
}

func (server *Server) verifyCandidates(
	ctx context.Context,
	candidates []database.RecommendationCandidateInput,
) ([]database.RecommendationCandidateInput, error) {
	if server.verifier == nil || len(candidates) == 0 {
		return candidates, nil
	}

	verified := make([]database.RecommendationCandidateInput, 0, len(candidates))
	for _, candidate := range candidates {
		result, ok, err := server.verifier.Verify(ctx, candidate)
		if err != nil {
			return nil, fmt.Errorf("Unable to verify recommendation candidates right now: %w", err)
		}
		if !ok {
			continue
		}
		result.Rank = len(verified) + 1
		verified = append(verified, result)
	}
	return verified, nil
}

func (server *Server) resolveCandidateLinks(
	ctx context.Context,
	candidates []database.RecommendationCandidateInput,
) []database.RecommendationCandidateInput {
	if server.linkResolver == nil || len(candidates) == 0 {
		return candidates
	}

	resolved := make([]database.RecommendationCandidateInput, 0, len(candidates))
	for _, candidate := range candidates {
		streamingURL, err := server.linkResolver.Resolve(ctx, candidate)
		if err != nil {
			// Graceful degradation: keep the candidate without a streaming URL
			// so batch generation never fails on streaming lookups.
			resolved = append(resolved, candidate)
			continue
		}
		candidate.StreamingURL = strings.TrimSpace(streamingURL)
		resolved = append(resolved, candidate)
	}
	return resolved
}

func (server *Server) resolveSongLinks(
	ctx context.Context,
	candidates []database.RecommendationCandidateInput,
) []database.RecommendationCandidateInput {
	if server.songLinkResolver == nil {
		return candidates
	}
	resolved := make([]database.RecommendationCandidateInput, 0, len(candidates))
	for _, candidate := range candidates {
		link, err := server.songLinkResolver.ResolveSong(ctx, candidate)
		if err != nil {
			resolved = append(resolved, candidate)
			continue
		}
		candidate.StreamingURL = strings.TrimSpace(link.URL)
		candidate.StreamingProvider = strings.TrimSpace(link.Provider)
		candidate.StreamingAppURL = strings.TrimSpace(link.AppURL)
		if candidate.StreamingURL == "" {
			candidate.StreamingProvider = ""
		}
		resolved = append(resolved, candidate)
	}
	return resolved
}

type OllamaRecommender struct {
	baseURL string
	model   string
	client  *http.Client
}

type MCPDiscoveryProvider struct {
	db *sql.DB
}

type MCPGroundedOllamaRecommender struct {
	ollama    *OllamaRecommender
	discovery DiscoveryProvider
	similar   mcpserver.SimilarArtistSource
	db        *sql.DB
}

// WithSimilarArtists attaches a real similar-artist source (normally Last.fm)
// used to anchor discovery on artists similar to the user's top affinity
// artists, so recommendations are grounded in compositional adjacency rather
// than guessed genre tags alone.
func (recommender *MCPGroundedOllamaRecommender) WithSimilarArtists(source mcpserver.SimilarArtistSource) *MCPGroundedOllamaRecommender {
	recommender.similar = source
	return recommender
}

type modelDiscoveryPlan struct {
	VibeSummary         string                     `json:"vibe_summary"`
	RequiredTraits      []string                   `json:"required_traits"`
	FlexibleTraits      []string                   `json:"flexible_traits"`
	ReferenceAnchors    []string                   `json:"reference_anchors"`
	ComparisonTraits    []string                   `json:"comparison_traits"`
	FalseFriendTraits   []string                   `json:"false_friend_traits"`
	BridgeTraits        []string                   `json:"bridge_traits"`
	ComparisonModifiers []string                   `json:"comparison_modifiers"`
	TargetVibe          string                     `json:"target_vibe"`
	FallbackTags        []string                   `json:"fallback_tags"`
	References          []recommendation.Reference `json:"references,omitempty"`
}

type modelCandidateSelection struct {
	CandidateIndex int      `json:"candidate_index"`
	Note           string   `json:"note"`
	Fit            string   `json:"fit"`
	EvidenceIDs    []string `json:"evidence_ids,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}

type modelCandidateSelectionResponse struct {
	Reply      string                    `json:"reply"`
	Selections []modelCandidateSelection `json:"selections"`
}

type MusicBrainzAlbumVerifier struct {
	baseURL      string
	client       *http.Client
	requestDelay time.Duration

	requestMu sync.Mutex
	lastCall  time.Time
}

type MusicBrainzReleaseRadar struct {
	baseURL         string
	listenBrainzURL string
	iTunesBaseURL   string
	client          *http.Client
	requestDelay    time.Duration
	cacheTTL        time.Duration

	requestMu sync.Mutex
	lastCall  time.Time

	cacheMu    sync.Mutex
	cache      releaseRadarCache
	refreshing bool
}

type releaseRadarCache struct {
	key         string
	releases    []NewRelease
	err         error
	refreshedAt time.Time
}

type musicBrainzReleaseGroupSearchResponse struct {
	ReleaseGroups []musicBrainzReleaseGroup `json:"release-groups"`
}

type musicBrainzReleaseGroup struct {
	ID               string                    `json:"id"`
	Title            string                    `json:"title"`
	FirstReleaseDate string                    `json:"first-release-date"`
	PrimaryType      string                    `json:"primary-type"`
	ArtistCredit     []musicBrainzArtistCredit `json:"artist-credit"`
	Tags             []musicBrainzTag          `json:"tags"`
}

type musicBrainzArtistCredit struct {
	Name       string `json:"name"`
	JoinPhrase string `json:"joinphrase"`
	Artist     struct {
		Name string `json:"name"`
	} `json:"artist"`
}

type musicBrainzTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type musicBrainzReleaseBrowseResponse struct {
	Releases []musicBrainzRelease `json:"releases"`
}

type musicBrainzRelease struct {
	Title  string              `json:"title"`
	Date   string              `json:"date"`
	Status string              `json:"status"`
	Media  []musicBrainzMedium `json:"media"`
}

type musicBrainzMedium struct {
	Tracks []musicBrainzTrack `json:"tracks"`
}

type musicBrainzTrack struct {
	Title     string `json:"title"`
	Position  int    `json:"position"`
	Recording struct {
		Title string `json:"title"`
	} `json:"recording"`
}

type listenBrainzFreshReleaseResponse struct {
	Payload struct {
		Releases []listenBrainzFreshRelease `json:"releases"`
	} `json:"payload"`
}

type listenBrainzFreshRelease struct {
	ArtistCreditName        string   `json:"artist_credit_name"`
	ReleaseName             string   `json:"release_name"`
	ReleaseDate             string   `json:"release_date"`
	ReleaseGroupMBID        string   `json:"release_group_mbid"`
	ReleaseGroupPrimaryType string   `json:"release_group_primary_type"`
	ReleaseTags             []string `json:"release_tags"`
}

type listenBrainzReleaseCandidate struct {
	release         NewRelease
	releaseGroupID  string
	artistForLookup string
	albumForLookup  string
}

type iTunesSearchResponse struct {
	Results []iTunesSearchResult `json:"results"`
}

type iTunesSearchResult struct {
	TrackID           int64  `json:"trackId"`
	ArtistName        string `json:"artistName"`
	TrackName         string `json:"trackName"`
	CollectionName    string `json:"collectionName"`
	PrimaryGenreName  string `json:"primaryGenreName"`
	TrackViewURL      string `json:"trackViewUrl"`
	CollectionViewURL string `json:"collectionViewUrl"`
}

type AppleMusicLinker struct {
	baseURL      string
	client       *http.Client
	requestDelay time.Duration

	requestMu sync.Mutex
	lastCall  time.Time
}

func NewAppleMusicLinker() *AppleMusicLinker {
	return &AppleMusicLinker{
		baseURL:      iTunesSearchBaseURL,
		client:       &http.Client{Timeout: defaultLinkerTimeout},
		requestDelay: defaultVerifyDelay,
	}
}

// AppleMusicAppURL returns the legacy Music app deep-link form while keeping
// the canonical HTTPS URL as the durable/browser-compatible destination.
func AppleMusicAppURL(webURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(webURL))
	if err != nil || !strings.EqualFold(parsed.Host, "music.apple.com") {
		return ""
	}
	parsed.Scheme = "itmss"
	return parsed.String()
}

func (linker *AppleMusicLinker) Resolve(ctx context.Context, candidate database.RecommendationCandidateInput) (string, error) {
	if linker == nil || linker.client == nil {
		return "", nil
	}

	targetCleanArtist, targetCleanAlbum, err := database.NormalizeAlbumLookup(candidate.Artist, candidate.Album)
	if err != nil {
		return "", err
	}
	if targetCleanArtist == "" || targetCleanAlbum == "" {
		return "", nil
	}

	endpoint, err := url.Parse(linker.baseURL)
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("term", strings.Join([]string{candidate.Artist, candidate.Album}, " "))
	query.Set("media", "music")
	query.Set("entity", "album")
	query.Set("limit", "10")
	endpoint.RawQuery = query.Encode()

	if err := linker.waitForRateLimit(ctx); err != nil {
		return "", err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", webUserAgent)

	response, err := linker.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("iTunes returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	var payload iTunesSearchResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload); err != nil {
		return "", err
	}
	for _, result := range payload.Results {
		resultCleanArtist, resultCleanAlbum, err := database.NormalizeAlbumLookup(result.ArtistName, result.CollectionName)
		if err != nil {
			return "", err
		}
		if resultCleanArtist != targetCleanArtist || !equivalentAlbumTitle(targetCleanAlbum, resultCleanAlbum) {
			continue
		}
		return strings.TrimSpace(result.CollectionViewURL), nil
	}
	return "", nil
}

func (linker *AppleMusicLinker) ResolveSong(ctx context.Context, candidate database.RecommendationCandidateInput) (SongLink, error) {
	if linker == nil || linker.client == nil {
		return SongLink{}, nil
	}
	targetSong := strings.TrimSpace(candidate.Song)
	if targetSong == "" {
		targetSong = strings.TrimSpace(candidate.StarterTrack)
	}
	targetArtist, _, err := database.NormalizeAlbumLookup(candidate.Artist, "placeholder")
	if err != nil || targetArtist == "" || targetSong == "" {
		return SongLink{}, err
	}
	endpoint, err := url.Parse(linker.baseURL)
	if err != nil {
		return SongLink{}, err
	}
	query := endpoint.Query()
	query.Set("term", strings.Join([]string{candidate.Artist, targetSong}, " "))
	query.Set("media", "music")
	query.Set("entity", "song")
	query.Set("limit", "10")
	endpoint.RawQuery = query.Encode()
	if err := linker.waitForRateLimit(ctx); err != nil {
		return SongLink{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return SongLink{}, err
	}
	request.Header.Set("User-Agent", webUserAgent)
	response, err := linker.client.Do(request)
	if err != nil {
		return SongLink{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return SongLink{}, fmt.Errorf("iTunes returned %s", response.Status)
	}
	var payload iTunesSearchResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload); err != nil {
		return SongLink{}, err
	}
	_, cleanSong, err := database.NormalizeAlbumLookup("placeholder", targetSong)
	if err != nil {
		return SongLink{}, err
	}
	for _, result := range payload.Results {
		resultArtist, _, err := database.NormalizeAlbumLookup(result.ArtistName, "placeholder")
		if err != nil {
			return SongLink{}, err
		}
		_, resultSong, err := database.NormalizeAlbumLookup("placeholder", result.TrackName)
		if err != nil {
			return SongLink{}, err
		}
		if resultArtist == targetArtist && resultSong == cleanSong && strings.TrimSpace(result.TrackViewURL) != "" {
			webURL := strings.TrimSpace(result.TrackViewURL)
			return SongLink{URL: webURL, AppURL: AppleMusicAppURL(webURL), TrackID: strconv.FormatInt(result.TrackID, 10), Provider: "apple_music"}, nil
		}
	}
	return SongLink{}, nil
}

type YouTubeSongLinker struct{}

func (YouTubeSongLinker) ResolveSong(_ context.Context, candidate database.RecommendationCandidateInput) (SongLink, error) {
	song := strings.TrimSpace(candidate.Song)
	if song == "" {
		song = strings.TrimSpace(candidate.StarterTrack)
	}
	artist := strings.TrimSpace(candidate.Artist)
	if artist == "" || song == "" {
		return SongLink{}, nil
	}
	return SongLink{URL: "https://www.youtube.com/results?search_query=" + url.QueryEscape(artist+" "+song), Provider: "youtube"}, nil
}

type FallbackSongLinkResolver struct {
	AppleMusic SongLinkResolver
	YouTube    SongLinkResolver
}

func (resolver FallbackSongLinkResolver) ResolveSong(ctx context.Context, candidate database.RecommendationCandidateInput) (SongLink, error) {
	if resolver.AppleMusic != nil {
		link, err := resolver.AppleMusic.ResolveSong(ctx, candidate)
		if err == nil && strings.TrimSpace(link.URL) != "" {
			return link, nil
		}
	}
	if resolver.YouTube != nil {
		link, err := resolver.YouTube.ResolveSong(ctx, candidate)
		if err == nil {
			return link, nil
		}
	}
	return SongLink{}, nil
}

func (linker *AppleMusicLinker) waitForRateLimit(ctx context.Context) error {
	if linker.requestDelay <= 0 {
		return nil
	}

	linker.requestMu.Lock()
	defer linker.requestMu.Unlock()

	if !linker.lastCall.IsZero() {
		wait := linker.requestDelay - time.Since(linker.lastCall)
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	linker.lastCall = time.Now()
	return nil
}

func NewMusicBrainzAlbumVerifier() *MusicBrainzAlbumVerifier {
	return &MusicBrainzAlbumVerifier{
		baseURL:      musicBrainzBaseURL,
		client:       &http.Client{Timeout: defaultVerifyTimeout},
		requestDelay: defaultVerifyDelay,
	}
}

func NewMusicBrainzReleaseRadar() *MusicBrainzReleaseRadar {
	return &MusicBrainzReleaseRadar{
		baseURL:         musicBrainzBaseURL,
		listenBrainzURL: listenBrainzBaseURL,
		iTunesBaseURL:   iTunesSearchBaseURL,
		client:          &http.Client{Timeout: defaultRadarTimeout},
		requestDelay:    defaultVerifyDelay,
		cacheTTL:        defaultRadarCacheTTL,
	}
}

func (radar *MusicBrainzReleaseRadar) NewReleases(
	ctx context.Context,
	artists []database.ArtistAffinity,
	since time.Time,
	until time.Time,
	limit int,
) ([]NewRelease, error) {
	if radar == nil || radar.client == nil {
		return nil, errors.New("MusicBrainz release radar is not configured.")
	}
	if limit <= 0 {
		limit = defaultRadarReleases
	}
	if since.IsZero() || until.IsZero() || since.After(until) {
		return nil, errors.New("release radar requires a valid date window.")
	}
	if radar.cacheTTL > 0 {
		return radar.cachedNewReleases(artists, since, until, limit)
	}

	return radar.fetchNewReleases(ctx, artists, since, until, limit)
}

func (radar *MusicBrainzReleaseRadar) cachedNewReleases(
	artists []database.ArtistAffinity,
	since time.Time,
	until time.Time,
	limit int,
) ([]NewRelease, error) {
	key := releaseRadarCacheKey(artists, since, until, limit)
	now := time.Now()

	radar.cacheMu.Lock()
	cache := radar.cache
	ttl := radar.cacheTTL
	if cache.err != nil {
		ttl = defaultRadarErrorTTL
	}
	if cache.key == key && !cache.refreshedAt.IsZero() && now.Sub(cache.refreshedAt) < ttl {
		releases := cloneNewReleases(cache.releases)
		err := cache.err
		radar.cacheMu.Unlock()
		return releases, err
	}
	if radar.refreshing {
		releases := cloneNewReleases(cache.releases)
		radar.cacheMu.Unlock()
		if cache.key == key && len(releases) > 0 {
			return releases, nil
		}
		return nil, errReleaseRadarRefreshInProgress
	}

	radar.refreshing = true
	releases := cloneNewReleases(cache.releases)
	hasStaleCache := cache.key == key && len(releases) > 0
	radar.cacheMu.Unlock()

	go radar.refreshNewReleaseCache(key, artists, since, until, limit)
	if hasStaleCache {
		return releases, nil
	}
	return nil, errReleaseRadarRefreshInProgress
}

func (radar *MusicBrainzReleaseRadar) refreshNewReleaseCache(
	key string,
	artists []database.ArtistAffinity,
	since time.Time,
	until time.Time,
	limit int,
) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRadarRefreshTimeout)
	defer cancel()

	releases, err := radar.fetchNewReleases(ctx, artists, since, until, limit)
	radar.cacheMu.Lock()
	defer radar.cacheMu.Unlock()
	radar.cache = releaseRadarCache{
		key:         key,
		releases:    cloneNewReleases(releases),
		err:         err,
		refreshedAt: time.Now(),
	}
	radar.refreshing = false
}

func (radar *MusicBrainzReleaseRadar) fetchNewReleases(
	ctx context.Context,
	artists []database.ArtistAffinity,
	since time.Time,
	until time.Time,
	limit int,
) ([]NewRelease, error) {
	if strings.TrimSpace(radar.listenBrainzURL) != "" {
		return radar.listenBrainzNewReleases(ctx, artists, since, until, limit)
	}

	return radar.musicBrainzNewReleases(ctx, artists, since, until, limit)
}

func (radar *MusicBrainzReleaseRadar) musicBrainzNewReleases(
	ctx context.Context,
	artists []database.ArtistAffinity,
	since time.Time,
	until time.Time,
	limit int,
) ([]NewRelease, error) {
	if len(artists) > defaultMusicBrainzRadarArtists {
		artists = artists[:defaultMusicBrainzRadarArtists]
	}
	candidates := make([]NewRelease, 0, limit)
	seen := make(map[string]bool)
	var firstErr error
	for _, artist := range artists {
		if len(candidates) >= limit {
			break
		}
		artistName := strings.TrimSpace(artist.Artist)
		if artistName == "" {
			continue
		}

		releases, err := radar.searchReleaseGroupsInWindow(ctx, artistName, since, until)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, release := range releases {
			cleanArtist, cleanAlbum, err := database.NormalizeAlbumLookup(release.Artist, release.Album)
			if err != nil || cleanArtist == "" || cleanAlbum == "" {
				continue
			}
			key := cleanArtist + "\x00" + cleanAlbum
			if seen[key] {
				continue
			}
			seen[key] = true
			candidates = append(candidates, release)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return releaseDateSortKey(candidates[i].ReleaseDate) > releaseDateSortKey(candidates[j].ReleaseDate)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	if firstErr != nil {
		return candidates, firstErr
	}
	return candidates, nil
}

func (radar *MusicBrainzReleaseRadar) listenBrainzNewReleases(
	ctx context.Context,
	artists []database.ArtistAffinity,
	since time.Time,
	until time.Time,
	limit int,
) ([]NewRelease, error) {
	baseURL := strings.TrimSpace(radar.listenBrainzURL)
	if baseURL == "" {
		return nil, errors.New("ListenBrainz release radar is not configured.")
	}

	endpoint, err := url.JoinPath(strings.TrimRight(baseURL, "/"), "explore", "fresh-releases")
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("days", strconv.Itoa(min(newReleaseWindowDays, 90)))
	query.Set("past", "true")
	query.Set("future", "false")
	query.Set("release_date", until.Format("2006-01-02"))
	query.Set("sort", "release_date")
	endpoint += "?" + query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", webUserAgent)

	response, err := radar.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("ListenBrainz returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	var payload listenBrainzFreshReleaseResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, listenBrainzResponseLimit)).Decode(&payload); err != nil {
		return nil, err
	}

	artistSet := affinityArtistSet(artists)
	candidates := make([]listenBrainzReleaseCandidate, 0, limit)
	seen := make(map[string]bool)
	for _, release := range payload.Payload.Releases {
		if !strings.EqualFold(strings.TrimSpace(release.ReleaseGroupPrimaryType), "album") {
			continue
		}
		releaseDate := strings.TrimSpace(release.ReleaseDate)
		releaseTime, ok := parseMusicBrainzDate(releaseDate)
		if !ok || releaseTime.Before(dayStart(since)) || releaseTime.After(dayStart(until)) {
			continue
		}
		cleanArtist, cleanAlbum, err := database.NormalizeAlbumLookup(release.ArtistCreditName, release.ReleaseName)
		if err != nil || cleanArtist == "" || cleanAlbum == "" || !artistMatchesAffinitySet(release.ArtistCreditName, cleanArtist, artistSet) {
			continue
		}
		key := cleanArtist + "\x00" + cleanAlbum
		if seen[key] {
			continue
		}
		seen[key] = true

		genreTags := cleanReleaseTags(release.ReleaseTags, 4)
		artistName := strings.TrimSpace(release.ArtistCreditName)
		albumName := strings.TrimSpace(release.ReleaseName)
		candidates = append(candidates, listenBrainzReleaseCandidate{
			release: NewRelease{
				Artist:      artistName,
				Album:       albumName,
				ReleaseDate: releaseDate,
				ReleaseType: release.ReleaseGroupPrimaryType,
				GenreTags:   genreTags,
				Blurb:       releaseBlurb(genreTags),
			},
			releaseGroupID:  release.ReleaseGroupMBID,
			artistForLookup: artistName,
			albumForLookup:  albumName,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return releaseDateSortKey(candidates[i].release.ReleaseDate) > releaseDateSortKey(candidates[j].release.ReleaseDate)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	releases := make([]NewRelease, 0, len(candidates))
	for _, candidate := range candidates {
		release := candidate.release
		if len(release.GenreTags) == 0 {
			release.GenreTags, _ = radar.releaseGroupTags(ctx, candidate.releaseGroupID)
		}
		if len(release.GenreTags) == 0 {
			release.GenreTags, _ = radar.iTunesAlbumTags(ctx, candidate.artistForLookup, candidate.albumForLookup)
		}
		release.Blurb = releaseBlurb(release.GenreTags)
		releases = append(releases, release)
	}
	return releases, nil
}

func (radar *MusicBrainzReleaseRadar) searchReleaseGroupsInWindow(
	ctx context.Context,
	artistName string,
	since time.Time,
	until time.Time,
) ([]NewRelease, error) {
	targetCleanArtist, err := normalizedArtistName(artistName)
	if err != nil {
		return nil, err
	}
	if targetCleanArtist == "" {
		return nil, nil
	}

	start := since.Format("2006-01-02")
	end := until.Format("2006-01-02")
	query := url.Values{}
	query.Set("fmt", "json")
	query.Set("limit", "5")
	query.Set("query", fmt.Sprintf(`artist:%s AND primarytype:album AND firstreleasedate:[%s TO %s]`, musicBrainzPhrase(artistName), start, end))

	endpoint, err := url.JoinPath(strings.TrimRight(radar.baseURL, "/"), "release-group")
	if err != nil {
		return nil, err
	}
	endpoint += "?" + query.Encode()

	if err := radar.waitForRateLimit(ctx); err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", webUserAgent)

	response, err := radar.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("MusicBrainz returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	var payload musicBrainzReleaseGroupSearchResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, err
	}

	releases := make([]NewRelease, 0, len(payload.ReleaseGroups))
	for _, releaseGroup := range payload.ReleaseGroups {
		if !strings.EqualFold(strings.TrimSpace(releaseGroup.PrimaryType), "album") {
			continue
		}
		releaseDate := strings.TrimSpace(releaseGroup.FirstReleaseDate)
		releaseTime, ok := parseMusicBrainzDate(releaseDate)
		if !ok || releaseTime.Before(dayStart(since)) || releaseTime.After(dayStart(until)) {
			continue
		}
		releaseArtist := artistCreditName(releaseGroup.ArtistCredit)
		releaseCleanArtist, err := normalizedArtistName(releaseArtist)
		if err != nil {
			return nil, err
		}
		if releaseCleanArtist != targetCleanArtist {
			continue
		}
		genreTags, err := radar.releaseGroupTags(ctx, releaseGroup.ID)
		if err != nil {
			genreTags = topMusicBrainzTags(releaseGroup.Tags, 4)
		}
		if len(genreTags) == 0 {
			genreTags, _ = radar.iTunesAlbumTags(ctx, releaseArtist, releaseGroup.Title)
		}
		releases = append(releases, NewRelease{
			Artist:      releaseArtist,
			Album:       strings.TrimSpace(releaseGroup.Title),
			ReleaseDate: releaseDate,
			ReleaseType: releaseGroup.PrimaryType,
			GenreTags:   genreTags,
			Blurb:       releaseBlurb(genreTags),
		})
	}
	return releases, nil
}

func (radar *MusicBrainzReleaseRadar) releaseGroupTags(ctx context.Context, releaseGroupID string) ([]string, error) {
	releaseGroupID = strings.TrimSpace(releaseGroupID)
	if releaseGroupID == "" {
		return nil, nil
	}

	endpoint, err := url.JoinPath(strings.TrimRight(radar.baseURL, "/"), "release-group", releaseGroupID)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("fmt", "json")
	query.Set("inc", "tags")
	endpoint += "?" + query.Encode()

	if err := radar.waitForRateLimit(ctx); err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", webUserAgent)

	response, err := radar.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("MusicBrainz returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	var releaseGroup musicBrainzReleaseGroup
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&releaseGroup); err != nil {
		return nil, err
	}
	return topMusicBrainzTags(releaseGroup.Tags, 4), nil
}

func (radar *MusicBrainzReleaseRadar) iTunesAlbumTags(ctx context.Context, artist string, album string) ([]string, error) {
	baseURL := strings.TrimSpace(radar.iTunesBaseURL)
	if baseURL == "" {
		return nil, nil
	}
	targetCleanArtist, targetCleanAlbum, err := database.NormalizeAlbumLookup(artist, album)
	if err != nil {
		return nil, err
	}
	if targetCleanArtist == "" || targetCleanAlbum == "" {
		return nil, nil
	}

	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("term", strings.Join([]string{artist, album}, " "))
	query.Set("media", "music")
	query.Set("entity", "album")
	query.Set("limit", "10")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", webUserAgent)

	response, err := radar.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("iTunes returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	var payload iTunesSearchResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	for _, result := range payload.Results {
		resultCleanArtist, resultCleanAlbum, err := database.NormalizeAlbumLookup(result.ArtistName, result.CollectionName)
		if err != nil {
			return nil, err
		}
		if resultCleanArtist != targetCleanArtist || !equivalentAlbumTitle(targetCleanAlbum, resultCleanAlbum) {
			continue
		}
		genre := strings.ToLower(strings.TrimSpace(result.PrimaryGenreName))
		if genre == "" || !isUsefulMusicBrainzTag(genre) {
			return nil, nil
		}
		return []string{genre}, nil
	}
	return nil, nil
}

func (radar *MusicBrainzReleaseRadar) waitForRateLimit(ctx context.Context) error {
	if radar.requestDelay <= 0 {
		return nil
	}

	radar.requestMu.Lock()
	defer radar.requestMu.Unlock()

	if !radar.lastCall.IsZero() {
		wait := radar.requestDelay - time.Since(radar.lastCall)
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	radar.lastCall = time.Now()
	return nil
}

func (verifier *MusicBrainzAlbumVerifier) Verify(
	ctx context.Context,
	candidate database.RecommendationCandidateInput,
) (database.RecommendationCandidateInput, bool, error) {
	cleanArtist, cleanAlbum, err := database.NormalizeAlbumLookup(candidate.Artist, candidate.Album)
	if err != nil {
		return candidate, false, err
	}
	if cleanArtist == "" || cleanAlbum == "" {
		return candidate, false, nil
	}

	response, err := verifier.searchReleaseGroups(ctx, candidate.Artist, candidate.Album)
	if err != nil {
		return candidate, false, err
	}

	for _, releaseGroup := range response.ReleaseGroups {
		releaseArtist := artistCreditName(releaseGroup.ArtistCredit)
		releaseCleanArtist, _, err := database.NormalizeAlbumLookup(releaseArtist, releaseGroup.Title)
		if err != nil {
			return candidate, false, err
		}
		if releaseCleanArtist != cleanArtist || !equivalentAlbumTitle(candidate.Album, releaseGroup.Title) {
			continue
		}
		if !isAlbumLikeReleaseGroup(releaseGroup.PrimaryType) {
			continue
		}

		candidate.Album = strings.TrimSpace(releaseGroup.Title)
		candidate.Artist = strings.TrimSpace(releaseArtist)
		if candidate.ReleaseYear == 0 {
			candidate.ReleaseYear = releaseYear(releaseGroup.FirstReleaseDate)
		}
		if len(candidate.GenreTags) == 0 {
			candidate.GenreTags = topMusicBrainzTags(releaseGroup.Tags, 6)
		}
		tracks, err := verifier.releaseGroupTracks(ctx, releaseGroup.ID)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return candidate, false, err
			}
			candidate.StarterTrack = ""
		} else {
			candidate.StarterTrack = verifiedStarterTrack(candidate.StarterTrack, tracks)
		}
		return candidate, true, nil
	}

	return candidate, false, nil
}

func (verifier *MusicBrainzAlbumVerifier) searchReleaseGroups(
	ctx context.Context,
	artist string,
	album string,
) (musicBrainzReleaseGroupSearchResponse, error) {
	query := url.Values{}
	query.Set("fmt", "json")
	query.Set("limit", "10")
	query.Set("query", fmt.Sprintf("%s AND artist:%s", musicBrainzReleaseGroupQuery(album), musicBrainzPhrase(artist)))

	endpoint, err := url.JoinPath(strings.TrimRight(verifier.baseURL, "/"), "release-group")
	if err != nil {
		return musicBrainzReleaseGroupSearchResponse{}, err
	}
	endpoint += "?" + query.Encode()

	if err := verifier.waitForRateLimit(ctx); err != nil {
		return musicBrainzReleaseGroupSearchResponse{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return musicBrainzReleaseGroupSearchResponse{}, err
	}
	request.Header.Set("User-Agent", webUserAgent)

	response, err := verifier.client.Do(request)
	if err != nil {
		return musicBrainzReleaseGroupSearchResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return musicBrainzReleaseGroupSearchResponse{}, fmt.Errorf("MusicBrainz returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	var payload musicBrainzReleaseGroupSearchResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload); err != nil {
		return musicBrainzReleaseGroupSearchResponse{}, err
	}
	return payload, nil
}

func (verifier *MusicBrainzAlbumVerifier) releaseGroupTracks(ctx context.Context, releaseGroupID string) ([]string, error) {
	releaseGroupID = strings.TrimSpace(releaseGroupID)
	if releaseGroupID == "" {
		return nil, nil
	}

	query := url.Values{}
	query.Set("fmt", "json")
	query.Set("limit", "10")
	query.Set("release-group", releaseGroupID)
	query.Set("inc", "media+recordings")

	endpoint, err := url.JoinPath(strings.TrimRight(verifier.baseURL, "/"), "release")
	if err != nil {
		return nil, err
	}
	endpoint += "?" + query.Encode()

	if err := verifier.waitForRateLimit(ctx); err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", webUserAgent)

	response, err := verifier.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("MusicBrainz returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	var payload musicBrainzReleaseBrowseResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, err
	}

	for _, release := range payload.Releases {
		if !strings.EqualFold(strings.TrimSpace(release.Status), "official") {
			continue
		}
		if tracks := releaseTrackTitles(release); len(tracks) > 0 {
			return tracks, nil
		}
	}
	for _, release := range payload.Releases {
		if tracks := releaseTrackTitles(release); len(tracks) > 0 {
			return tracks, nil
		}
	}
	return nil, nil
}

func (verifier *MusicBrainzAlbumVerifier) waitForRateLimit(ctx context.Context) error {
	if verifier.requestDelay <= 0 {
		return nil
	}

	verifier.requestMu.Lock()
	defer verifier.requestMu.Unlock()

	if !verifier.lastCall.IsZero() {
		wait := verifier.requestDelay - time.Since(verifier.lastCall)
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	verifier.lastCall = time.Now()
	return nil
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Stream   bool            `json:"stream"`
	Format   string          `json:"format,omitempty"`
	Think    *bool           `json:"think,omitempty"`
	Messages []ollamaMessage `json:"messages"`
}

type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
	Error   string        `json:"error,omitempty"`
}

type modelRecommendationCandidate struct {
	Artist       string   `json:"artist"`
	Album        string   `json:"album"`
	StarterTrack string   `json:"starter_track"`
	ReleaseYear  int      `json:"release_year"`
	GenreTags    []string `json:"genre_tags"`
	Note         string   `json:"note"`
}

type modelRecommendationResponse struct {
	Reply      string                         `json:"reply"`
	Candidates []modelRecommendationCandidate `json:"candidates"`
}

func (recommender *OllamaRecommender) Recommend(ctx context.Context, request RecommendationRequest, profile ProfileContext) (RecommendationDraft, error) {
	content, err := recommender.chat(ctx, recommendationSystemPrompt(), recommendationUserPrompt(request, profile))
	if err != nil {
		return RecommendationDraft{}, err
	}

	var modelResponse modelRecommendationResponse
	if err := json.Unmarshal([]byte(extractJSONObject(content)), &modelResponse); err != nil {
		return RecommendationDraft{}, fmt.Errorf("decode model recommendation JSON: %w", err)
	}

	draft := RecommendationDraft{
		Reply: strings.TrimSpace(modelResponse.Reply),
	}
	for idx, candidate := range modelResponse.Candidates {
		draft.Candidates = append(draft.Candidates, database.RecommendationCandidateInput{
			Artist:       candidate.Artist,
			Album:        candidate.Album,
			StarterTrack: candidate.StarterTrack,
			ReleaseYear:  candidate.ReleaseYear,
			GenreTags:    candidate.GenreTags,
			Rank:         idx + 1,
			Note:         candidate.Note,
		})
	}

	return draft, nil
}

func (recommender *OllamaRecommender) chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if strings.TrimSpace(recommender.baseURL) == "" {
		return "", errors.New("Ollama URL is not configured.")
	}
	if strings.TrimSpace(recommender.model) == "" {
		return "", errors.New("Ollama model is not configured.")
	}

	think := false
	body, err := json.Marshal(ollamaChatRequest{
		Model:  recommender.model,
		Stream: false,
		Format: "json",
		Think:  &think,
		Messages: []ollamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	})
	if err != nil {
		return "", err
	}

	endpoint, err := url.JoinPath(recommender.baseURL, "/api/chat")
	if err != nil {
		return "", err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	httpResponse, err := recommender.client.Do(httpRequest)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", errors.New("Ollama did not finish before the local web timeout. Try again, use a smaller model, or start music-vault web with a larger --ollama-timeout value.")
		}
		return "", fmt.Errorf("Unable to reach Ollama at %s. Is the local model server running?", recommender.baseURL)
	}
	defer httpResponse.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(httpResponse.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return "", fmt.Errorf("Ollama returned %s: %s", httpResponse.Status, strings.TrimSpace(string(responseBody)))
	}

	var ollamaResponse ollamaChatResponse
	if err := json.Unmarshal(responseBody, &ollamaResponse); err != nil {
		return "", fmt.Errorf("decode Ollama response: %w", err)
	}
	if ollamaResponse.Error != "" {
		return "", errors.New(ollamaResponse.Error)
	}

	return ollamaResponse.Message.Content, nil
}

func (provider *MCPDiscoveryProvider) Discover(
	ctx context.Context,
	request DiscoveryRequest,
) ([]mcpserver.DiscoveryCandidate, error) {
	if provider == nil || provider.db == nil {
		return nil, errors.New("MCP discovery provider is not configured.")
	}

	result, err := mcpserver.GetVerifiedDiscoveryCandidates(ctx, &database.DB{Ctx: provider.db}, mcpserver.VerifiedDiscoveryQuery{
		TargetVibe:                   request.TargetVibe,
		FallbackTags:                 request.FallbackTags,
		SeedArtists:                  request.SeedArtists,
		Limit:                        request.Limit,
		SkipAlbumGenreReconciliation: strings.EqualFold(strings.TrimSpace(request.Mode), "song"),
	})
	if err != nil {
		return nil, err
	}
	return result.Candidates, nil
}

func (recommender *MCPGroundedOllamaRecommender) Recommend(
	ctx context.Context,
	request RecommendationRequest,
	profile ProfileContext,
) (RecommendationDraft, error) {
	if recommender == nil || recommender.ollama == nil {
		return RecommendationDraft{}, errors.New("Ollama recommender is not configured.")
	}
	if recommender.discovery == nil {
		return RecommendationDraft{}, errors.New("MCP discovery provider is not configured.")
	}

	planStarted := time.Now()
	plan, err := recommender.planDiscovery(ctx, request, profile)
	if err != nil {
		return RecommendationDraft{}, err
	}
	planMetric := recommendation.StageMetric{Stage: "planning", ElapsedMS: time.Since(planStarted).Milliseconds(), ExternalCalls: 1}
	if len(plan.FallbackTags) == 0 && strings.TrimSpace(plan.TargetVibe) == "" {
		plan.TargetVibe = request.Message
	}

	similarityStarted := time.Now()
	seedArtists := similaritySeedsForPlan(plan.References, profile.Artists)
	similarityEdges := mcpserver.SimilarArtistEdges(ctx, recommender.similar, seedArtists)
	discoverySeeds := similarArtistsFromEdges(similarityEdges, plan.References, profile.Artists)
	neighborTerms := make([]string, 0, len(plan.References)*4+len(discoverySeeds))
	for _, reference := range plan.References {
		neighborTerms = append(neighborTerms, reference.Artist, reference.Album, reference.Song, reference.SuppliedText)
	}
	neighborTerms = append(neighborTerms, discoverySeeds...)
	if recommender.db != nil {
		if neighborContext, contextErr := database.FetchRelevantRecommendationContext(ctx, recommender.db, neighborTerms, 24); contextErr == nil {
			profile.RelevantContext = append(profile.RelevantContext, neighborContext...)
		}
	}
	similarityMetric := recommendation.StageMetric{Stage: "similarity", ElapsedMS: time.Since(similarityStarted).Milliseconds(), ExternalCalls: len(seedArtists)}

	discoveryStarted := time.Now()
	candidates, err := recommender.discovery.Discover(ctx, DiscoveryRequest{
		TargetVibe:   plan.TargetVibe,
		FallbackTags: compactWebStrings(plan.FallbackTags),
		SeedArtists:  discoverySeeds,
		Limit:        discoveryCandidateFetchLimit(request.Mode, request.Limit),
		Mode:         request.Mode,
	})
	if err != nil {
		return RecommendationDraft{}, fmt.Errorf("MCP verified discovery failed: %w", err)
	}
	discoveryMetric := recommendation.StageMetric{Stage: "discovery", ElapsedMS: time.Since(discoveryStarted).Milliseconds(), ExternalCalls: 1}
	diagnostics := RecommendationDiagnostics{
		RequestedCount:     clampBatchLimit(request.Mode, request.Limit),
		DiscoveryPoolCount: len(candidates),
	}
	if len(candidates) == 0 {
		return RecommendationDraft{}, errors.New("MCP verified discovery returned no candidates for those tags. Try a slightly broader prompt.")
	}
	beforeAvoid := len(candidates)
	candidates = filterAvoidedDiscoveryCandidates(candidates, requestAvoidTags(request))
	diagnostics.DiscoveryAvoidedCount = beforeAvoid - len(candidates)
	if len(candidates) == 0 {
		return RecommendationDraft{}, errors.New("MCP verified discovery returned no candidates after applying avoid filters. Try a slightly broader prompt or fewer avoided styles.")
	}
	candidates = rankDiscoveryCandidatesForPrompt(request, plan, candidates)
	candidates = rankDiscoveryCandidatesWithFeedback(request, plan, candidates, profile.RecentFeedback)

	selectionStarted := time.Now()
	draft, err := recommender.selectDiscoveryCandidates(ctx, request, profile, plan, candidates)
	draft.Diagnostics.Stages = append([]recommendation.StageMetric{planMetric, similarityMetric, discoveryMetric}, draft.Diagnostics.Stages...)
	draft.Diagnostics.Stages = appendStageMetric(draft.Diagnostics.Stages, "selection", selectionStarted, 1)
	diagnostics.ModelSelectedCount = draft.Diagnostics.ModelSelectedCount
	diagnostics.BackfillAddedCount = draft.Diagnostics.BackfillAddedCount
	diagnostics.Stages = draft.Diagnostics.Stages
	draft.References = append([]recommendation.Reference(nil), plan.References...)
	draft.SimilarityEdges = similarityEdges
	draft.Providers = []recommendation.ProviderStatus{{Provider: "last.fm", Status: similarityProviderStatus(recommender.similar, seedArtists, similarityEdges), Calls: len(seedArtists)}}
	draft.Diagnostics = diagnostics
	return draft, err
}

func similarityProviderStatus(source mcpserver.SimilarArtistSource, seeds []string, edges []recommendation.SimilarityEdge) string {
	if len(edges) > 0 {
		return "available"
	}
	if source == nil {
		return "unconfigured"
	}
	if len(seeds) == 0 {
		return "not_requested"
	}
	return "no_results"
}

func (recommender *MCPGroundedOllamaRecommender) planDiscovery(
	ctx context.Context,
	request RecommendationRequest,
	profile ProfileContext,
) (modelDiscoveryPlan, error) {
	content, err := recommender.ollama.chat(ctx, discoveryPlanSystemPrompt(), discoveryPlanUserPrompt(request, profile))
	if err != nil {
		return modelDiscoveryPlan{}, err
	}

	var plan modelDiscoveryPlan
	if err := json.Unmarshal([]byte(extractJSONObject(content)), &plan); err != nil {
		return modelDiscoveryPlan{}, fmt.Errorf("decode model discovery plan JSON: %w", err)
	}
	plan.VibeSummary = strings.TrimSpace(plan.VibeSummary)
	plan.RequiredTraits = compactWebStrings(plan.RequiredTraits)
	plan.FlexibleTraits = compactWebStrings(plan.FlexibleTraits)
	plan.ReferenceAnchors = compactWebStrings(plan.ReferenceAnchors)
	plan.ComparisonTraits = compactWebStrings(plan.ComparisonTraits)
	plan.FalseFriendTraits = compactWebStrings(plan.FalseFriendTraits)
	plan.BridgeTraits = compactWebStrings(plan.BridgeTraits)
	plan.ComparisonModifiers = compactWebStrings(plan.ComparisonModifiers)
	plan.TargetVibe = strings.TrimSpace(plan.TargetVibe)
	plan.FallbackTags = compactWebStrings(plan.FallbackTags)
	plan = calibrateDiscoveryPlanForPrompt(request, plan)
	plan.References = reconcileRecommendationReferences(plan.References, request.Examples)
	return plan, nil
}

// similarSeedArtists derives the artist-names used to anchor discovery from the
// user's top affinity artists via the configured real similar-artist source
// (Last.fm), excluding anything already in the local library. It returns nil
// when no similar source is configured so discovery falls back to genre tags.
func (recommender *MCPGroundedOllamaRecommender) similarSeedArtists(
	ctx context.Context,
	artists []database.ArtistAffinity,
) []string {
	if recommender.similar == nil || len(artists) == 0 {
		return nil
	}

	exclude := make(map[string]bool, len(artists))
	var seeds []string
	for _, affinity := range artists {
		clean, err := utils.NormalizeSearchText(affinity.Artist)
		if err != nil || clean == "" {
			continue
		}
		exclude[clean] = true
		if len(seeds) < 4 {
			seeds = append(seeds, affinity.Artist)
		}
	}
	if len(seeds) == 0 {
		return nil
	}
	return mcpserver.SimilarArtistNames(ctx, recommender.similar, seeds, exclude, 4)
}

func similaritySeedsForPlan(references []recommendation.Reference, artists []database.ArtistAffinity) []string {
	seeds := make([]string, 0, 4)
	seen := make(map[string]bool)
	for _, reference := range references {
		if !strings.EqualFold(strings.TrimSpace(reference.Polarity), "positive") {
			continue
		}
		name := strings.TrimSpace(reference.Artist)
		if name == "" {
			name = strings.TrimSpace(reference.SuppliedText)
		}
		clean, err := utils.NormalizeSearchText(name)
		if err != nil || clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		seeds = append(seeds, name)
		if len(seeds) == 4 {
			return seeds
		}
	}
	if len(seeds) > 0 {
		return seeds
	}
	for _, affinity := range artists {
		clean, err := utils.NormalizeSearchText(affinity.Artist)
		if err != nil || clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		seeds = append(seeds, affinity.Artist)
		if len(seeds) == 4 {
			break
		}
	}
	return seeds
}

func similarArtistsFromEdges(edges []recommendation.SimilarityEdge, references []recommendation.Reference, artists []database.ArtistAffinity) []string {
	if len(edges) == 0 {
		return nil
	}
	exclude := make(map[string]bool)
	for _, reference := range references {
		if clean, err := utils.NormalizeSearchText(reference.Artist); err == nil && clean != "" {
			exclude[clean] = true
		}
	}
	for _, artist := range artists {
		if clean, err := utils.NormalizeSearchText(artist.Artist); err == nil && clean != "" {
			exclude[clean] = true
		}
	}
	seen := make(map[string]bool)
	result := make([]string, 0, 4)
	for _, edge := range edges {
		name := strings.TrimSpace(edge.Neighbor)
		clean, err := utils.NormalizeSearchText(name)
		if err != nil || clean == "" || seen[clean] || exclude[clean] {
			continue
		}
		seen[clean] = true
		result = append(result, name)
		if len(result) == 4 {
			break
		}
	}
	return result
}

func (recommender *MCPGroundedOllamaRecommender) selectDiscoveryCandidates(
	ctx context.Context,
	request RecommendationRequest,
	profile ProfileContext,
	plan modelDiscoveryPlan,
	candidates []mcpserver.DiscoveryCandidate,
) (RecommendationDraft, error) {
	limit := clampBatchLimit(request.Mode, request.Limit)
	content, err := recommender.ollama.chat(
		ctx,
		discoverySelectionSystemPrompt(),
		discoverySelectionUserPrompt(request, profile, plan, candidates, limit),
	)
	if err != nil {
		draft := fallbackDiscoveryDraft(request, candidates, plan, limit)
		draft.DegradedReason = "selection_model_error"
		return draft, nil
	}

	var selection modelCandidateSelectionResponse
	if err := json.Unmarshal([]byte(extractJSONObject(content)), &selection); err != nil {
		draft := fallbackDiscoveryDraft(request, candidates, plan, limit)
		draft.DegradedReason = "selection_malformed_json"
		return draft, nil
	}

	draft := RecommendationDraft{Reply: strings.TrimSpace(selection.Reply)}
	seen := make(map[int]bool)
	for _, selected := range selection.Selections {
		idx := selected.CandidateIndex - 1
		if idx < 0 || idx >= len(candidates) || seen[idx] {
			continue
		}
		seen[idx] = true
		qualification := qualifyDiscoveryCandidate(request, plan, candidates[idx])
		assessment := validateCandidateSelectionAssessment(selected, candidates[idx])
		if !assessment.Valid {
			continue
		}
		if qualification.Status != recommendation.QualificationSupported && qualification.Status != recommendation.QualificationPlausible {
			continue
		}
		candidateInput := discoveryCandidateInput(
			candidates[idx],
			len(draft.Candidates)+1,
			assessment.Note,
		)
		if songRecommendationMode(request) {
			candidateInput.Song = candidates[idx].TrackName
		}
		draft.Candidates = append(draft.Candidates, candidateInput)
	}
	draft.Diagnostics.ModelSelectedCount = len(draft.Candidates)
	var backfillAdded int
	draft.Candidates, backfillAdded = backfillRecommendationCandidates(request, draft.Candidates, candidates, plan, limit, seen)
	draft.Diagnostics.BackfillAddedCount = backfillAdded
	if len(draft.Candidates) > limit {
		draft.Candidates = draft.Candidates[:limit]
		resetRecommendationCandidateRanks(draft.Candidates)
	}
	if len(draft.Candidates) == 0 {
		draft := fallbackDiscoveryDraft(request, candidates, plan, limit)
		draft.DegradedReason = "selection_no_qualified_candidates"
		return draft, nil
	}
	if draft.Reply == "" {
		draft.Reply = "Here are verified albums from the MCP discovery search."
	}
	return draft, nil
}

type candidateSelectionAssessment struct {
	Valid       bool
	Fit         string
	EvidenceIDs []string
	Note        string
}

func validateCandidateSelectionAssessment(selection modelCandidateSelection, candidate mcpserver.DiscoveryCandidate) candidateSelectionAssessment {
	fit := strings.ToLower(strings.TrimSpace(selection.Fit))
	if fit == "" {
		fit = recommendation.QualificationPlausible
	}
	if fit != recommendation.QualificationSupported && fit != recommendation.QualificationPlausible {
		return candidateSelectionAssessment{}
	}
	evidence := make(map[string]recommendation.Evidence, len(candidate.Evidence))
	for _, item := range candidate.Evidence {
		if item.ID != "" {
			evidence[item.ID] = item
		}
	}
	for _, id := range selection.EvidenceIDs {
		item, ok := evidence[id]
		if !ok || item.EntityScope == "" || item.Claim == "" {
			return candidateSelectionAssessment{}
		}
		if item.EntityScope == recommendation.EntityRecording && strings.TrimSpace(candidate.TrackName) == "" {
			return candidateSelectionAssessment{}
		}
	}
	if fit == recommendation.QualificationSupported && len(selection.EvidenceIDs) == 0 {
		return candidateSelectionAssessment{}
	}
	note := strings.TrimSpace(selection.Note)
	if note == "" {
		note = "Candidate assessed as " + fit + "."
	}
	return candidateSelectionAssessment{Valid: true, Fit: fit, EvidenceIDs: append([]string(nil), selection.EvidenceIDs...), Note: note}
}

func qualifyDiscoveryCandidate(request RecommendationRequest, plan modelDiscoveryPlan, candidate mcpserver.DiscoveryCandidate) recommendation.Qualification {
	performanceRequested := promptRequestsVirtuosicPlaying(strings.ToLower(strings.Join([]string{request.Message, request.Mood}, " ")))
	if len(candidate.Evidence) == 0 {
		if performanceRequested || len(plan.References) > 0 || promptRequestsComparison(strings.ToLower(request.Message)) {
			return recommendation.Qualification{Status: recommendation.QualificationInsufficient, Reason: "missing_candidate_evidence"}
		}
		return recommendation.Qualification{Status: recommendation.QualificationPlausible, Reason: "legacy_broad_tag_compatibility"}
	}
	for _, evidence := range candidate.Evidence {
		claim := strings.ToLower(strings.Join([]string{evidence.Claim, evidence.Details}, " "))
		if evidence.Kind == recommendation.EvidenceLocalFit && strings.Contains(claim, "negative") {
			return recommendation.Qualification{Status: recommendation.QualificationContradicted, Reason: "contradictory_local_fit", EvidenceIDs: []string{evidence.ID}}
		}
	}
	if performanceRequested && !scopedPerformanceEvidenceSupports(candidate, "virtuosic/lead-playing") {
		return recommendation.Qualification{Status: recommendation.QualificationInsufficient, Reason: "performance_trait_unverified"}
	}
	if songRecommendationMode(request) && strings.TrimSpace(candidate.TrackName) == "" {
		return recommendation.Qualification{Status: recommendation.QualificationInsufficient, Reason: "track_identity_missing"}
	}
	if len(plan.References) > 0 || promptRequestsComparison(strings.ToLower(request.Message)) {
		return recommendation.Qualification{Status: recommendation.QualificationPlausible, Reason: "verified_metadata_and_reference_path", EvidenceIDs: evidenceIDs(candidate.Evidence)}
	}
	return recommendation.Qualification{Status: recommendation.QualificationPlausible, Reason: "broad_tag_compatibility", EvidenceIDs: evidenceIDs(candidate.Evidence)}
}

func evidenceIDs(evidence []recommendation.Evidence) []string {
	ids := make([]string, 0, len(evidence))
	for _, item := range evidence {
		if item.ID != "" {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func discoveryPlanSystemPrompt() string {
	return `You interpret personal music recommendation prompts into a musical discovery plan, then map that plan to canonical MusicBrainz genre/tag searches.
Return only JSON with this exact shape:
{"vibe_summary":"short musical intent","required_traits":["trait"],"flexible_traits":["trait"],"reference_anchors":["artist or style"],"comparison_traits":["trait"],"false_friend_traits":["trait"],"bridge_traits":["trait"],"comparison_modifiers":["modifier"],"target_vibe":"","fallback_tags":["tag one","tag two"],"references":[{"supplied_text":"artist or project","artist":"","album":"","song":"","entity_scope":"artist","polarity":"positive","origin":"extracted_text"}]}

Rules:
- Treat the user's words as evidence of the intended listening feel, not a literal checklist, unless they use explicit hard constraints like "must", "only", "no", or "avoid".
- Infer musical intent across energy, rhythm feel, texture, density, vocal/lyric importance, novelty, and mood before choosing tags.
- Put true hard requirements in required_traits. Put vibe cues, loose descriptors, and "or" alternatives in flexible_traits.
- For comparison prompts using named artists/styles or phrases like "like", "similar to", "same vibe as", "but heavier", "less", or "more", decompose the reference before choosing tags.
- Put named comparison artists/styles in reference_anchors.
- Put the musical dimensions the user likely wants from the comparison in comparison_traits.
- Put superficial broad matches that could miss the requested tone in false_friend_traits.
- Put indirect but acceptable matches in bridge_traits.
- Put comparative changes such as heavier, lighter, less harsh, more electronic, more melodic, less death metal, or more groove-focused in comparison_modifiers.
- Treat requested performance features such as virtuosic, virtuoso, shredding, lead playing, neoclassical playing, Symphony X, or Children of Bodom as core musical intent. Put them in required_traits unless the user clearly frames them as optional.
- For prompts like "heavy or funky or both", do not require every candidate to be both heavy and funky; search broadly enough to find strong candidates from either side, while preferring overlap.
- Prefer fallback_tags with 3 to 8 lower-case genre, style, or scene tags likely to exist in MusicBrainz.
- Use target_vibe only when the prompt is already one canonical tag.
- Active prompt ingredients outrank the user's historical genre profile. Use profile context only for calibration, never to erase requested traits.
- Comparison intent outranks broad genre overlap. For example, Rage Against the Machine implies funk metal, rap metal, alternative metal, rhythmic groove, and staccato riffs before generic heaviness; KNOWER implies jazz-funk/electronic/fusion/groove traits; Opeth with "less death metal" should preserve progressive, melodic, atmospheric, and dynamic traits while de-emphasizing death metal.
- Do not add metal/heavy tags merely because the user's profile is metal-heavy. Only include metal/heavy tags when the current prompt asks for metal, heavy, sludge, death, thrash, doom, grind, or adjacent weight.
- For cross-genre prompts, include each important vibe ingredient as separate tags instead of collapsing everything into one phrase.
- When the prompt references non-metal artists/styles such as KNOWER, jazz-funk, funk, fusion, electronic, country, bluegrass, or dubstep, search those spaces directly instead of translating them into metal-adjacent tags.
- When the prompt says "funk", "funky", or "groove", include tags such as "funk", "jazz-funk", "funk rock", "electro-funk", or "dance-punk"; use "funk metal" only if metal/heavy is also requested.
- When the prompt asks for "bass lines", "bassline", "low end", or rhythm-section feel, use genre proxies such as "funk", "jazz-funk", "dub", "post-punk", "dance-punk", "electro-funk", or "jazz fusion"; MusicBrainz tags rarely encode bass performance directly.
- When the prompt names KNOWER or similar jazz/electronic fusion acts, include tags such as "jazz-funk", "jazz fusion", "electropop", "synth-pop", and "funk".
- When the prompt says "weird", "experimental", or "left-field", include tags such as "avant-garde metal", "experimental rock", "noise rock", "math rock", or "art rock" when musically compatible.
- When the prompt says "heavy", include specific heavy tags that preserve any other requested ingredients, such as "funk metal", "alternative metal", "progressive metal", "death metal", or "sludge metal".
- When the prompt asks for virtuosic metal like Symphony X or Children of Bodom, prefer tags such as "neoclassical metal", "power metal", "progressive metal", "symphonic metal", and "melodic metal"; use "melodic death metal" as a secondary bridge, not the whole search.
- Do not output artist names, album names, vague adjectives, moods, or prose in fallback_tags.
- Avoid tags that recent feedback says are not aligned with the user.`
}

func discoverySelectionSystemPrompt() string {
	return `You rank verified MCP discovery candidates for a personal music recommendation batch.
Return only JSON with this exact shape:
{"reply":"short plain-language summary","selections":[{"candidate_index":1,"fit":"plausible","evidence_ids":["catalog_identity"],"reason":"short reason","note":"one concise reason"}]}

Rules:
- candidate_index must be one of the numbered candidates provided by the user message.
- Do not create, rename, or substitute artists, albums, or tracks.
	- Recommend albums by default; when the request asks for songs, recommend the listed verified track as the song candidate while retaining its album as context.
- Present the listed track as a matched entry point, not necessarily the album's best opener or most representative song.
- Use vibe_summary, required_traits, and flexible_traits as the main intent. Treat MusicBrainz tags as evidence, not the full meaning of the prompt.
- Prefer high-impact matches to the user's current prompt over generic taste-anchor similarity.
- Treat taste anchors as guardrails and quality calibration, not as target genres. Do not pull the batch toward metal unless the current prompt asks for metal/heavy.
- Preserve non-metal request terms such as jazz, funk, groove, electronic, country, dubstep, weird, or experimental even when the user's profile has strong metal affinities.
- Prefer one album per artist unless the prompt explicitly requests a catalog, discography, deep-dive, or multiple albums by the same artist.
- Use comparison anchors, comparison traits, false friends, bridge traits, and modifiers as ranking guidance. Do not treat a named reference artist as just a broad genre label.
- Rank candidates matching comparison_traits above candidates that only match broad genre, scene, or heaviness overlap. Treat false_friend_traits as weak or negative evidence unless bridge_traits or comparison_traits compensate.
- Prefer candidates whose genre_tags cover multiple prompt ingredients over candidates that only match the broadest or most familiar tag.
- Use supported_prompt_traits and unsupported_prompt_traits exactly as provided. Do not claim an unsupported trait in a note.
- If the prompt asks for funk/funky/groove, rank candidates with supported_prompt_traits containing funk/groove above candidates where that trait is unsupported.
- If the prompt asks for virtuosic playing, shredding, neoclassical leads, Symphony X, or Children of Bodom, rank candidates with supported_prompt_traits containing virtuosic/lead-playing above generic melodic death or progressive metal candidates that do not support that trait.
- If prompt_trait_logic says any-of, candidates may satisfy either side of an "or" request; describe only their supported traits.
- Never mention unsupported_prompt_traits, unsupported traits, or support-status bookkeeping in the user-facing reply or notes.
- Keep each note to one short sentence and do not invent sourcing, reviews, credits, direct reference-artist similarity, instrumentation, or track details.`
}

func recommendationSystemPrompt() string {
	return `You generate music recommendations for a local personal library.
Return only JSON with this exact shape:
{"reply":"short plain-language summary","candidates":[{"artist":"Artist","album":"Album","song":"Song when song mode is requested","starter_track":"Track to sample","release_year":2024,"genre_tags":["tag"],"note":"one concise reason"}]}

Rules:
- Recommend albums by default. If the user requests individual songs, recommend verified song candidates and keep the album as context.
- Include 4 to 6 candidates unless the user asks for fewer.
- Prefer dense, personality-forward, technically interesting music.
- For hip-hop, default to English unless the non-English record is production-forward enough to overcome the lyric barrier.
- Treat the local artist affinity list as taste calibration and avoidance context, not as a source of albums to recommend back.
- Do not recommend artists or albums shown in local context or recent feedback.
- Treat disliked feedback as "not aligned with this user", not as a claim that the album is bad.
- Avoid soft atmospheric art-electronic unless the user explicitly asks for it.
- Keep notes short and do not invent albums, track titles, credits, or fake sourcing.`
}

func recommendationUserPrompt(request RecommendationRequest, profile ProfileContext) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "User prompt: %s\n", request.Message)
	if request.Mood != "" {
		fmt.Fprintf(&builder, "Mood/context: %s\n", request.Mood)
	}
	if request.Avoid != "" {
		fmt.Fprintf(&builder, "Avoid: %s\n", request.Avoid)
	}
	fmt.Fprintf(&builder, "Candidate limit: %d\n\n", request.Limit)

	builder.WriteString("Top local artist affinities:\n")
	for _, artist := range profile.Artists {
		fmt.Fprintf(&builder, "- %s: favorites=%d, dislikes=%d, avg_rating=%s, affinity=%.2f\n",
			artist.Artist,
			artist.FavoriteTracksCount,
			artist.DislikedTracksCount,
			formatNullableFloat(artist.AvgUserRating),
			artist.CurvedAffinityScore,
		)
	}

	builder.WriteString("\nTop local genre topography:\n")
	for _, genre := range profile.Genres {
		fmt.Fprintf(&builder, "- %s: total=%d, favorites=%d, dislikes=%d, avg_album_rating=%s\n",
			genre.Subgenre,
			genre.TotalTracks,
			genre.FavoriteTracksCount,
			genre.DislikedTracksCount,
			formatNullableFloat(genre.AvgAlbumRating),
		)
	}

	builder.WriteString("\nRecent recommendation feedback:\n")
	if len(profile.RecentFeedback) == 0 {
		builder.WriteString("- none\n")
	} else {
		for _, feedback := range profile.RecentFeedback {
			fmt.Fprintf(&builder, "- %s by %s: %s", feedback.Album, feedback.Artist, feedback.Verdict)
			if feedback.Notes != "" {
				fmt.Fprintf(&builder, " (%s)", feedback.Notes)
			}
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

func discoveryPlanUserPrompt(request RecommendationRequest, profile ProfileContext) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "User prompt: %s\n", request.Message)
	if request.Mood != "" {
		fmt.Fprintf(&builder, "Mood/context: %s\n", request.Mood)
	}
	if request.Avoid != "" {
		fmt.Fprintf(&builder, "Avoid: %s\n", request.Avoid)
	}
	if len(request.Examples) > 0 {
		builder.WriteString("\nExplicit user examples (authoritative; preserve polarity and identity):\n")
		for _, example := range request.Examples {
			fmt.Fprintf(&builder, "- scope=%s polarity=%s supplied=%q artist=%q album=%q song=%q note=%q\n", example.EntityScope, example.Polarity, example.SuppliedText, example.Artist, example.Album, example.Song, example.Notes)
		}
	}
	builder.WriteString("Planning priority: infer the intended musical feel before choosing tags. The user's current prompt owns the search space; profile data calibrates quality and novelty but must not drag every prompt back to the user's top genres. Do not treat casual wording as a literal checklist unless the user states a hard constraint. If the prompt asks for non-metal jazz, funk, bass-forward, electronic, country, bluegrass, or dubstep, search those spaces directly. If it asks for new music with great bass lines that is heavy or funky or both, infer a search for prominent low-end/rhythm-section energy, groove, and weight; heavy-only, funky-only, and overlapping candidates may all be valid. If it asks for bass lines, low end, or rhythm-section feel, include groove/bass proxy tags such as funk, jazz-funk, dub, post-punk, dance-punk, electro-funk, or jazz fusion. If it contains weird/experimental, include an experimental or avant-garde tag.\n")
	appendRelevantContextPrompt(&builder, profile.RelevantContext)

	builder.WriteString("\nTop genre signals:\n")
	for idx, genre := range profile.Genres {
		if idx == 10 {
			break
		}
		fmt.Fprintf(&builder, "- %s: favorites=%d, total=%d\n",
			genre.Subgenre,
			genre.FavoriteTracksCount,
			genre.TotalTracks,
		)
	}

	builder.WriteString("\nRecent feedback to avoid repeating:\n")
	if len(profile.RecentFeedback) == 0 {
		builder.WriteString("- none\n")
	} else {
		for idx, feedback := range profile.RecentFeedback {
			if idx == 12 {
				break
			}
			fmt.Fprintf(&builder, "- %s by %s: %s", feedback.Album, feedback.Artist, feedback.Verdict)
			if feedback.Notes != "" {
				fmt.Fprintf(&builder, " (%s)", feedback.Notes)
			}
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

func discoverySelectionUserPrompt(
	request RecommendationRequest,
	profile ProfileContext,
	plan modelDiscoveryPlan,
	candidates []mcpserver.DiscoveryCandidate,
	limit int,
) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "User prompt: %s\n", request.Message)
	if request.Mood != "" {
		fmt.Fprintf(&builder, "Mood/context: %s\n", request.Mood)
	}
	if request.Avoid != "" {
		fmt.Fprintf(&builder, "Avoid: %s\n", request.Avoid)
	}
	appendRelevantContextPrompt(&builder, profile.RelevantContext)
	fmt.Fprintf(&builder, "Prompt trait logic: %s\n", promptTraitLogic(request))
	if plan.VibeSummary != "" {
		fmt.Fprintf(&builder, "Interpreted vibe: %s\n", plan.VibeSummary)
	}
	if len(plan.RequiredTraits) > 0 {
		fmt.Fprintf(&builder, "Required traits: %s\n", strings.Join(plan.RequiredTraits, ", "))
	}
	if len(plan.References) > 0 {
		builder.WriteString("Effective references:\n")
		for _, reference := range plan.References {
			fmt.Fprintf(&builder, "- scope=%s polarity=%s origin=%s supplied=%q resolution=%s\n", reference.EntityScope, reference.Polarity, reference.Origin, reference.SuppliedText, reference.Resolution)
		}
	}
	if len(plan.FlexibleTraits) > 0 {
		fmt.Fprintf(&builder, "Flexible traits: %s\n", strings.Join(plan.FlexibleTraits, ", "))
	}
	if len(plan.ReferenceAnchors) > 0 {
		fmt.Fprintf(&builder, "Comparison anchors: %s\n", strings.Join(plan.ReferenceAnchors, ", "))
	}
	if len(plan.ComparisonTraits) > 0 {
		fmt.Fprintf(&builder, "Comparison traits: %s\n", strings.Join(plan.ComparisonTraits, ", "))
	}
	if len(plan.FalseFriendTraits) > 0 {
		fmt.Fprintf(&builder, "False-friend traits: %s\n", strings.Join(plan.FalseFriendTraits, ", "))
	}
	if len(plan.BridgeTraits) > 0 {
		fmt.Fprintf(&builder, "Bridge traits: %s\n", strings.Join(plan.BridgeTraits, ", "))
	}
	if len(plan.ComparisonModifiers) > 0 {
		fmt.Fprintf(&builder, "Comparison modifiers: %s\n", strings.Join(plan.ComparisonModifiers, ", "))
	}
	fmt.Fprintf(&builder, "Selection limit: %d\n", limit)
	if len(plan.FallbackTags) > 0 {
		fmt.Fprintf(&builder, "MCP fallback_tags used: %s\n", strings.Join(plan.FallbackTags, ", "))
	} else if plan.TargetVibe != "" {
		fmt.Fprintf(&builder, "MCP target_vibe used: %s\n", plan.TargetVibe)
	}

	builder.WriteString("\nTaste calibration artists, not recommendation targets:\n")
	for idx, artist := range profile.Artists {
		if idx == 8 {
			break
		}
		fmt.Fprintf(&builder, "- %s: affinity=%.2f, favorites=%d\n",
			artist.Artist,
			artist.CurvedAffinityScore,
			artist.FavoriteTracksCount,
		)
	}

	builder.WriteString("\nVerified MCP candidates. Select only by candidate_index:\n")
	for idx, candidate := range candidates {
		fmt.Fprintf(&builder, "%d. %s — %s — %s", idx+1, candidate.Artist, candidate.Album, candidate.TrackName)
		if candidate.ReleaseYear > 0 {
			fmt.Fprintf(&builder, " (%d)", candidate.ReleaseYear)
		}
		if candidate.Runtime != "" {
			fmt.Fprintf(&builder, "; runtime=%s", candidate.Runtime)
		}
		if len(candidate.GenreTags) > 0 {
			fmt.Fprintf(&builder, "; genre_tags=%s", strings.Join(candidate.GenreTags, ", "))
		}
		supported, unsupported := promptTraitCoverageForPlan(request, plan, candidate)
		if len(supported) > 0 {
			fmt.Fprintf(&builder, "; supported_prompt_traits=%s", strings.Join(supported, ", "))
		}
		if len(unsupported) > 0 {
			fmt.Fprintf(&builder, "; unsupported_prompt_traits=%s", strings.Join(unsupported, ", "))
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

func appendRelevantContextPrompt(builder *strings.Builder, records []database.RecommendationContextRecord) {
	if len(records) == 0 {
		return
	}
	builder.WriteString("\nRequest-relevant local context (signal type is authoritative; listening activity is not fit evidence):\n")
	for _, record := range records {
		fmt.Fprintf(builder, "- source=%s signal=%s entity=%s artist=%q album=%q song=%q value=%q", record.Source, record.SignalType, record.Entity, record.Artist, record.Album, record.Song, record.Value)
		if record.Details != "" {
			fmt.Fprintf(builder, " details=%q", record.Details)
		}
		builder.WriteString("\n")
	}
}

func decodeJSON(request *http.Request, target any) error {
	defer request.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func validateRecommendationExamples(examples []recommendation.Reference) error {
	if len(examples) > 16 {
		return errors.New("recommendation examples are limited to 16 entries")
	}
	for index, example := range examples {
		if strings.TrimSpace(example.SuppliedText) == "" && strings.TrimSpace(example.Artist) == "" {
			return fmt.Errorf("recommendation example %d requires supplied_text or artist", index+1)
		}
		scope := strings.ToLower(strings.TrimSpace(example.EntityScope))
		if scope != "artist" && scope != "album" && scope != "song" {
			return fmt.Errorf("recommendation example %d has invalid entity_scope", index+1)
		}
		polarity := strings.ToLower(strings.TrimSpace(example.Polarity))
		if polarity != "positive" && polarity != "negative" {
			return fmt.Errorf("recommendation example %d has invalid polarity", index+1)
		}
		if scope == "artist" && strings.TrimSpace(example.Song) != "" {
			return fmt.Errorf("recommendation example %d artist examples must not include song", index+1)
		}
		if scope == "album" && (strings.TrimSpace(example.Artist) == "" || strings.TrimSpace(example.Album) == "") {
			return fmt.Errorf("recommendation example %d album examples require artist and album", index+1)
		}
		if scope == "song" && (strings.TrimSpace(example.Artist) == "" || strings.TrimSpace(example.Album) == "" || strings.TrimSpace(example.Song) == "") {
			return fmt.Errorf("recommendation example %d song examples require artist, album, and song", index+1)
		}
	}
	return nil
}

func reconcileRecommendationReferences(extracted, explicit []recommendation.Reference) []recommendation.Reference {
	result := make([]recommendation.Reference, 0, len(extracted)+len(explicit))
	for _, reference := range extracted {
		reference = sanitizeReference(reference)
		reference.Origin = "extracted_text"
		reference.Polarity = normalizeReferencePolarity(reference.Polarity)
		reference.EntityScope = normalizeReferenceScope(reference.EntityScope)
		if strings.TrimSpace(reference.SuppliedText) != "" {
			result = append(result, reference)
		}
	}
	for _, example := range explicit {
		example = sanitizeReference(example)
		example.Origin = "structured_example"
		example.Polarity = normalizeReferencePolarity(example.Polarity)
		example.EntityScope = normalizeReferenceScope(example.EntityScope)
		key := referenceIdentityKey(example)
		replaced := false
		for index := range result {
			if key != "" && referenceIdentityKey(result[index]) == key {
				result[index] = example
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, example)
		}
	}
	return result
}

func sanitizeReference(reference recommendation.Reference) recommendation.Reference {
	reference.CanonicalName = ""
	reference.MBID = ""
	// Resolution and canonical identity are server-owned. Until a bounded
	// server-side lookup resolves this reference, client/model claims remain
	// explicitly unresolved.
	reference.Resolution = "unresolved"
	return reference
}

func normalizeReferencePolarity(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "negative") {
		return "negative"
	}
	return "positive"
}

func normalizeReferenceScope(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "album":
		return "album"
	case "song", "recording":
		return "song"
	default:
		return "artist"
	}
}

func referenceIdentityKey(reference recommendation.Reference) string {
	scope := normalizeReferenceScope(reference.EntityScope)
	if strings.TrimSpace(reference.MBID) != "" {
		clean, _ := utils.NormalizeSearchText(reference.MBID)
		return scope + "|mbid|" + clean
	}
	parts := []string{reference.Artist, reference.Album, reference.Song}
	if scope == "artist" {
		parts = []string{reference.Artist, reference.CanonicalName, reference.SuppliedText}
	}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		clean, err := utils.NormalizeSearchText(part)
		if err != nil {
			return ""
		}
		values = append(values, clean)
	}
	key := strings.Join(values, "|")
	if strings.Trim(key, "|") == "" {
		return ""
	}
	return scope + "|name|" + key
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeJSONError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}

func clampBatchLimit(mode string, limit int) int {
	if limit <= 0 {
		return defaultBatchLimit
	}
	maxLimit := maxAlbumBatchLimit
	if strings.EqualFold(strings.TrimSpace(mode), "song") || strings.EqualFold(strings.TrimSpace(mode), "songs") {
		maxLimit = maxSongBatchLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func discoveryCandidateFetchLimit(mode string, limit int) int {
	limit = clampBatchLimit(mode, limit) * 3
	if limit < 12 {
		return 12
	}
	return limit
}

func calibrateDiscoveryPlanForPrompt(
	request RecommendationRequest,
	plan modelDiscoveryPlan,
) modelDiscoveryPlan {
	promptText := strings.ToLower(strings.Join([]string{request.Message, request.Mood}, " "))
	if promptText == "" {
		return plan
	}

	derivedComparison := promptDerivedComparisonPlan(promptText)
	plan.ReferenceAnchors = mergeCompactWebStrings(plan.ReferenceAnchors, derivedComparison.ReferenceAnchors)
	plan.ComparisonTraits = mergeCompactWebStrings(plan.ComparisonTraits, derivedComparison.ComparisonTraits)
	plan.FalseFriendTraits = mergeCompactWebStrings(plan.FalseFriendTraits, derivedComparison.FalseFriendTraits)
	plan.BridgeTraits = mergeCompactWebStrings(plan.BridgeTraits, derivedComparison.BridgeTraits)
	plan.ComparisonModifiers = mergeCompactWebStrings(plan.ComparisonModifiers, derivedComparison.ComparisonModifiers)

	tags := compactWebStrings(plan.FallbackTags)
	derivedTags := promptDerivedDiscoveryTags(promptText)
	if promptRequestsHeavyMusic(promptText) {
		if promptRequestsVirtuosicPlaying(promptText) || promptRequestsComparison(promptText) {
			tags = prependDiscoveryTags(derivedTags, tags, 8)
		} else if len(tags) == 0 {
			tags = prependDiscoveryTags(derivedTags, nil, 8)
		}
	} else {
		tags = filterDiscoveryTags(tags, func(tag string) bool {
			return !isMetalDiscoveryTag(tag)
		})
		tags = prependDiscoveryTags(derivedTags, tags, 8)
	}
	avoidTags := requestAvoidTags(request)
	tags = filterAvoidedDiscoveryTags(tags, avoidTags)
	if len(tags) == 0 {
		tags = filterAvoidedDiscoveryTags(derivedTags, avoidTags)
	}
	if styleMatchesAnyAvoidTag(plan.TargetVibe, avoidTags) {
		plan.TargetVibe = ""
	}
	plan.FallbackTags = tags
	return plan
}

func mergeCompactWebStrings(left []string, right []string) []string {
	return compactWebStrings(append(append([]string(nil), left...), right...))
}

func promptRequestsComparison(promptText string) bool {
	promptText = " " + strings.Join(strings.Fields(strings.ToLower(promptText)), " ") + " "
	if containsAny(promptText, []string{
		" like ",
		" similar to ",
		" same vibe ",
		" vibes ",
		" in the style of ",
		" but ",
		" less ",
		" more ",
	}) {
		return true
	}
	return len(promptDerivedComparisonPlan(promptText).ReferenceAnchors) > 0
}

func promptRequestsHeavyMusic(promptText string) bool {
	return containsAny(promptText, []string{
		"metal",
		"heavy",
		"sludge",
		"death",
		"thrash",
		"doom",
		"black metal",
		"grind",
		"hardcore",
	})
}

func promptRequestsVirtuosicPlaying(promptText string) bool {
	return containsAny(promptText, []string{
		"virtuosic",
		"vituosic",
		"virtuoso",
		"shred",
		"shredding",
		"lead playing",
		"lead guitar",
		"neoclassical",
	})
}

func isMetalDiscoveryTag(tag string) bool {
	return containsAny(tag, []string{
		"metal",
		"death",
		"thrash",
		"doom",
		"sludge",
		"grind",
		"metalcore",
		"hardcore",
	})
}

func promptDerivedDiscoveryTags(promptText string) []string {
	var tags []string
	add := func(values ...string) {
		tags = append(tags, values...)
	}

	add(promptDerivedComparisonPlan(promptText).FallbackTags...)
	if promptRequestsVirtuosicPlaying(promptText) {
		add("neoclassical metal", "power metal", "progressive metal", "symphonic metal", "melodic metal")
	}
	if promptRequestsHeavyMusic(promptText) && containsAny(promptText, []string{"melodic", "melody"}) {
		add("melodic metal", "power metal", "melodic death metal")
	}
	if containsAny(promptText, []string{"knower", "louis cole", "clown core"}) {
		add("jazz-funk", "jazz fusion", "electropop", "synth-pop", "funk")
	}
	if containsAny(promptText, []string{"jazz", "fusion"}) {
		add("jazz-funk", "jazz fusion", "nu jazz", "funk")
	}
	if containsAny(promptText, []string{"funk", "funky", "groove", "groovy"}) {
		add("funk", "jazz-funk", "funk rock", "electro-funk")
	}
	if containsAny(promptText, []string{"bass", "bassline", "bass line", "low end", "rhythm section"}) {
		add("funk", "jazz-funk", "dub", "post-punk", "dance-punk")
	}
	if containsAny(promptText, []string{"electronic", "synth", "dubstep", "edm"}) {
		add("electronic", "electropop", "synth-pop", "idm", "dubstep")
	}
	if containsAny(promptText, []string{"country", "bluegrass", "americana", "billy strings", "orville peck"}) {
		add("bluegrass", "americana", "country")
	}
	if containsAny(promptText, []string{"eclectic", "weird", "experimental", "left-field", "left field"}) {
		add("art pop", "experimental", "art rock")
	}
	return compactWebStrings(tags)
}

func promptDerivedComparisonPlan(promptText string) modelDiscoveryPlan {
	promptText = strings.ToLower(promptText)
	var plan modelDiscoveryPlan
	addTraits := func(values ...string) {
		plan.ComparisonTraits = append(plan.ComparisonTraits, values...)
	}
	addBridgeTraits := func(values ...string) {
		plan.BridgeTraits = append(plan.BridgeTraits, values...)
	}
	addModifiers := func(values ...string) {
		plan.ComparisonModifiers = append(plan.ComparisonModifiers, values...)
	}

	if containsAny(promptText, []string{"but heavier", "heavier"}) {
		addModifiers("heavier")
		addBridgeTraits("added weight")
	}
	if containsAny(promptText, []string{"less harsh", "lighter"}) {
		addModifiers("less harsh")
	}
	if containsAny(promptText, []string{"more electronic", "electronic but", "more synth"}) {
		addModifiers("more electronic")
		addTraits("electronic", "synth")
	}
	if containsAny(promptText, []string{"more melodic", "melodic but"}) {
		addModifiers("more melodic")
		addTraits("melodic")
	}
	if containsAny(promptText, []string{"more groove", "groove-focused", "groove focused"}) {
		addModifiers("more groove-focused")
		addTraits("groove")
	}

	plan.ReferenceAnchors = compactWebStrings(plan.ReferenceAnchors)
	plan.ComparisonTraits = compactWebStrings(plan.ComparisonTraits)
	plan.FalseFriendTraits = compactWebStrings(plan.FalseFriendTraits)
	plan.BridgeTraits = compactWebStrings(plan.BridgeTraits)
	plan.ComparisonModifiers = compactWebStrings(plan.ComparisonModifiers)
	plan.FallbackTags = compactWebStrings(plan.FallbackTags)
	return plan
}

func requestAvoidTags(request RecommendationRequest) []string {
	return parseAvoidTags(request.Avoid)
}

func parseAvoidTags(value string) []string {
	seen := make(map[string]bool)
	var tags []string
	for _, raw := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	}) {
		clean := normalizeStyleText(raw)
		if clean == "" {
			continue
		}
		for _, tag := range expandAvoidTag(clean) {
			if tag == "" || seen[tag] {
				continue
			}
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

func expandAvoidTag(tag string) []string {
	values := []string{tag}
	switch tag {
	case "tech death", "technical death":
		values = append(values, "technical death metal")
	case "tech death metal", "technical death metal":
		values = append(values, "technical death")
	case "avant garde metal":
		values = append(values, "avant garde")
	}
	return compactWebStrings(values)
}

func normalizeStyleText(value string) string {
	clean, err := normalizeOptional(value)
	if err != nil {
		return ""
	}
	clean = strings.ReplaceAll(clean, "avante", "avant")
	clean = strings.ReplaceAll(clean, "tech death", "technical death")
	return strings.Join(strings.Fields(clean), " ")
}

func filterAvoidedDiscoveryTags(tags []string, avoidTags []string) []string {
	if len(tags) == 0 || len(avoidTags) == 0 {
		return tags
	}
	filtered := make([]string, 0, len(tags))
	for _, tag := range tags {
		if styleMatchesAnyAvoidTag(tag, avoidTags) {
			continue
		}
		filtered = append(filtered, tag)
	}
	return filtered
}

func filterAvoidedDiscoveryCandidates(
	candidates []mcpserver.DiscoveryCandidate,
	avoidTags []string,
) []mcpserver.DiscoveryCandidate {
	if len(candidates) == 0 || len(avoidTags) == 0 {
		return candidates
	}
	filtered := make([]mcpserver.DiscoveryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidateHasAvoidedTag(candidate.GenreTags, avoidTags) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func filterAvoidedRecommendationCandidates(
	candidates []database.RecommendationCandidateInput,
	avoidTags []string,
) []database.RecommendationCandidateInput {
	if len(candidates) == 0 || len(avoidTags) == 0 {
		return candidates
	}
	filtered := make([]database.RecommendationCandidateInput, 0, len(candidates))
	for _, candidate := range candidates {
		if candidateHasAvoidedTag(candidate.GenreTags, avoidTags) {
			continue
		}
		candidate.Rank = len(filtered) + 1
		filtered = append(filtered, candidate)
	}
	return filtered
}

func candidateHasAvoidedTag(candidateTags []string, avoidTags []string) bool {
	for _, tag := range candidateTags {
		if styleMatchesAnyAvoidTag(tag, avoidTags) {
			return true
		}
	}
	return false
}

func styleMatchesAnyAvoidTag(tag string, avoidTags []string) bool {
	for _, avoidTag := range avoidTags {
		if styleMatchesAvoidTag(tag, avoidTag) {
			return true
		}
	}
	return false
}

func styleMatchesAvoidTag(tag string, avoidTag string) bool {
	tag = normalizeStyleText(tag)
	avoidTag = normalizeStyleText(avoidTag)
	if tag == "" || avoidTag == "" {
		return false
	}
	if tag == avoidTag {
		return true
	}
	return containsPhrase(tag, avoidTag)
}

func containsPhrase(value string, phrase string) bool {
	value = " " + strings.Join(strings.Fields(value), " ") + " "
	phrase = " " + strings.Join(strings.Fields(phrase), " ") + " "
	return strings.Contains(value, phrase)
}

func filterDiscoveryTags(tags []string, keep func(string) bool) []string {
	filtered := make([]string, 0, len(tags))
	for _, tag := range tags {
		if keep(tag) {
			filtered = append(filtered, tag)
		}
	}
	return filtered
}

func prependDiscoveryTags(priority []string, rest []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	combined := make([]string, 0, limit)
	seen := make(map[string]bool, limit)
	for _, tag := range append(priority, rest...) {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		combined = append(combined, tag)
		if len(combined) == limit {
			break
		}
	}
	return combined
}

func compactWebStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func parseMusicBrainzDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func dayStart(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func releaseDateSortKey(value string) string {
	parsed, ok := parseMusicBrainzDate(value)
	if !ok {
		return "9999-12-31"
	}
	return parsed.Format("2006-01-02")
}

func releaseBlurb(tags []string) string {
	tags = compactWebStrings(tags)
	if len(tags) == 0 {
		return ""
	}
	if len(tags) > 3 {
		tags = tags[:3]
	}
	return "Tagged " + strings.Join(tags, ", ") + "."
}

func affinityArtistSet(artists []database.ArtistAffinity) map[string]bool {
	artistSet := make(map[string]bool, len(artists))
	for _, artist := range artists {
		cleanArtist := strings.TrimSpace(artist.CleanArtist)
		if cleanArtist == "" {
			cleanArtist, _ = normalizedArtistName(artist.Artist)
		}
		if cleanArtist != "" {
			artistSet[cleanArtist] = true
		}
		for _, component := range artistCreditComponents(artist.Artist) {
			cleanComponent, err := normalizedArtistName(component)
			if err == nil && cleanComponent != "" {
				artistSet[cleanComponent] = true
			}
		}
	}
	return artistSet
}

func releaseRadarAffinityArtists(artists []database.ArtistAffinity) []database.ArtistAffinity {
	filtered := make([]database.ArtistAffinity, 0, len(artists))
	for _, artist := range artists {
		if releaseRadarArtistEligible(artist) {
			filtered = append(filtered, artist)
		}
	}
	return filtered
}

func releaseRadarArtistEligible(artist database.ArtistAffinity) bool {
	if artist.CurvedAffinityScore > 0 || artist.FavoriteTracksCount > 0 {
		return true
	}
	return artist.AvgUserRating.Valid && artist.AvgUserRating.Float64 >= 4.0
}

func artistMatchesAffinitySet(artist string, cleanArtist string, artistSet map[string]bool) bool {
	if artistSet[cleanArtist] {
		return true
	}
	for _, component := range artistCreditComponents(artist) {
		cleanComponent, err := normalizedArtistName(component)
		if err == nil && artistSet[cleanComponent] {
			return true
		}
	}
	return false
}

func artistCreditComponents(artist string) []string {
	replacer := strings.NewReplacer(
		" feat. ", "|",
		" ft. ", "|",
		" featuring ", "|",
		" with ", "|",
		" x ", "|",
		" X ", "|",
		" / ", "|",
		" & ", "|",
		",", "|",
		";", "|",
	)
	artist = replacer.Replace(strings.TrimSpace(artist))
	parts := strings.Split(artist, "|")
	components := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		components = append(components, part)
	}
	return components
}

func cleanReleaseTags(tags []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	values := make([]string, 0, limit)
	seen := make(map[string]bool, limit)
	for _, tag := range compactWebStrings(tags) {
		if seen[tag] || !isUsefulMusicBrainzTag(tag) {
			continue
		}
		seen[tag] = true
		values = append(values, tag)
		if len(values) == limit {
			break
		}
	}
	return values
}

func normalizedArtistName(value string) (string, error) {
	cleanArtist, _, err := database.NormalizeAlbumLookup(value, "placeholder")
	if err != nil {
		return "", err
	}
	return cleanArtist, nil
}

func releaseRadarCacheKey(
	artists []database.ArtistAffinity,
	since time.Time,
	until time.Time,
	limit int,
) string {
	var builder strings.Builder
	builder.WriteString(since.Format("2006-01-02"))
	builder.WriteByte('|')
	builder.WriteString(until.Format("2006-01-02"))
	builder.WriteByte('|')
	builder.WriteString(strconv.Itoa(limit))
	for _, artist := range artists {
		cleanArtist := strings.TrimSpace(artist.CleanArtist)
		if cleanArtist == "" {
			cleanArtist, _ = normalizedArtistName(artist.Artist)
		}
		if cleanArtist == "" {
			continue
		}
		builder.WriteByte('|')
		builder.WriteString(cleanArtist)
	}
	return builder.String()
}

func cloneNewReleases(releases []NewRelease) []NewRelease {
	if len(releases) == 0 {
		return nil
	}
	clone := make([]NewRelease, len(releases))
	copy(clone, releases)
	for idx := range clone {
		clone[idx].GenreTags = append([]string(nil), releases[idx].GenreTags...)
	}
	return clone
}

func fallbackDiscoveryDraft(
	request RecommendationRequest,
	candidates []mcpserver.DiscoveryCandidate,
	plan modelDiscoveryPlan,
	limit int,
) RecommendationDraft {
	if limit <= 0 {
		limit = defaultBatchLimit
	}
	draft := RecommendationDraft{
		Reply: "Here are verified albums from the MCP discovery search.",
	}
	for idx, candidate := range candidates {
		if qualification := qualifyDiscoveryCandidate(request, plan, candidate); qualification.Status != recommendation.QualificationSupported && qualification.Status != recommendation.QualificationPlausible {
			continue
		}
		note := fallbackDiscoveryNote(request, plan, candidate)
		candidateInput := discoveryCandidateInput(candidate, idx+1, note)
		if songRecommendationMode(request) {
			candidateInput.Song = candidate.TrackName
		}
		draft.Candidates = append(draft.Candidates, candidateInput)
	}
	draft.Candidates = applyRecommendationBatchDiversity(request, draft.Candidates)
	if len(draft.Candidates) > limit {
		draft.Candidates = draft.Candidates[:limit]
		resetRecommendationCandidateRanks(draft.Candidates)
	}
	return draft
}

func fallbackDiscoveryNote(
	request RecommendationRequest,
	plan modelDiscoveryPlan,
	candidate mcpserver.DiscoveryCandidate,
) string {
	signals := comparisonSignalsForRequest(request, plan)
	if !comparisonSignalsEmpty(signals) {
		matchedTraits := supportedComparisonTraits(signals.ComparisonTraits, candidate.GenreTags)
		if len(matchedTraits) > 0 {
			return "Returned by MCP discovery with verified tags for " + strings.Join(firstStrings(matchedTraits, 2), ", ") + "."
		}
		matchedBridgeTraits := supportedComparisonTraits(signals.BridgeTraits, candidate.GenreTags)
		if len(matchedBridgeTraits) > 0 {
			return "Returned by MCP discovery through a related " + matchedBridgeTraits[0] + " bridge."
		}
		return "Returned by MCP discovery for the requested comparison."
	}
	if len(plan.FallbackTags) > 0 {
		return "Returned by MCP discovery for " + strings.Join(plan.FallbackTags, ", ") + "."
	}
	if plan.TargetVibe != "" {
		return "Returned by MCP discovery for " + plan.TargetVibe + "."
	}
	return "Verified by MCP discovery"
}

func applyRecommendationBatchDiversity(
	request RecommendationRequest,
	candidates []database.RecommendationCandidateInput,
) []database.RecommendationCandidateInput {
	return dedupeRecommendationCandidates(candidates, requestAllowsRepeatedArtists(request) || songRecommendationMode(request))
}

func backfillRecommendationCandidates(
	request RecommendationRequest,
	selected []database.RecommendationCandidateInput,
	pool []mcpserver.DiscoveryCandidate,
	plan modelDiscoveryPlan,
	limit int,
	selectedIndexes map[int]bool,
) ([]database.RecommendationCandidateInput, int) {
	if limit <= 0 {
		limit = defaultBatchLimit
	}
	result := applyRecommendationBatchDiversity(request, selected)
	if len(result) >= limit {
		return result, 0
	}
	backfillAdded := 0

	for idx, candidate := range pool {
		if selectedIndexes[idx] {
			continue
		}
		qualification := qualifyDiscoveryCandidate(request, plan, candidate)
		if qualification.Status != recommendation.QualificationSupported && qualification.Status != recommendation.QualificationPlausible {
			continue
		}
		candidateInput := discoveryCandidateInput(candidate, len(result)+1, fallbackDiscoveryNote(request, plan, candidate))
		if songRecommendationMode(request) {
			candidateInput.Song = candidate.TrackName
		}
		result = applyRecommendationBatchDiversity(request, append(result, candidateInput))
		if len(result) > len(selected)+backfillAdded {
			backfillAdded++
		}
		if len(result) >= limit {
			break
		}
	}
	return result, backfillAdded
}

func selectedDiscoveryIndexes(selected []database.RecommendationCandidateInput, pool []mcpserver.DiscoveryCandidate) map[int]bool {
	seen := make(map[string]bool, len(selected))
	for _, candidate := range selected {
		artist, album := recommendationCandidateKeys(candidate)
		seen[artist+"\x00"+album] = true
	}
	result := make(map[int]bool)
	for index, candidate := range pool {
		artist, album := recommendationCandidateKeys(database.RecommendationCandidateInput{Artist: candidate.Artist, Album: candidate.Album})
		if seen[artist+"\x00"+album] {
			result[index] = true
		}
	}
	return result
}

func requestAllowsRepeatedArtists(request RecommendationRequest) bool {
	text := " " + strings.Join(strings.Fields(strings.ToLower(strings.Join([]string{request.Message, request.Mood}, " "))), " ") + " "
	return containsAny(text, []string{
		" deep dive ",
		" deep-dive ",
		" catalog ",
		" catalogue ",
		" discography ",
		" multiple albums ",
		" several albums ",
		" more albums ",
		" multiple releases ",
		" several releases ",
		" more releases ",
		" same artist ",
		" same band ",
		" one artist ",
		" single artist ",
		" albums by ",
		" releases by ",
		" another album by ",
	})
}

func dedupeRecommendationCandidates(
	candidates []database.RecommendationCandidateInput,
	allowRepeatedArtists bool,
) []database.RecommendationCandidateInput {
	if len(candidates) == 0 {
		return candidates
	}

	seenArtists := make(map[string]bool, len(candidates))
	seenAlbums := make(map[string]bool, len(candidates))
	filtered := make([]database.RecommendationCandidateInput, 0, len(candidates))
	for _, candidate := range candidates {
		cleanArtist, cleanAlbum := recommendationCandidateKeys(candidate)
		if cleanArtist == "" {
			cleanArtist = strings.ToLower(strings.TrimSpace(candidate.Artist))
		}
		if cleanAlbum == "" {
			cleanAlbum = strings.ToLower(strings.TrimSpace(candidate.Album))
		}
		identity := cleanAlbum
		if strings.TrimSpace(candidate.Song) != "" {
			_, identity, _ = database.NormalizeAlbumLookup("placeholder", candidate.Song)
		}
		albumKey := cleanArtist + "\x00" + identity
		if albumKey != "\x00" && seenAlbums[albumKey] {
			continue
		}
		if !allowRepeatedArtists && cleanArtist != "" && seenArtists[cleanArtist] {
			continue
		}
		candidate.Rank = len(filtered) + 1
		filtered = append(filtered, candidate)
		if cleanArtist != "" {
			seenArtists[cleanArtist] = true
		}
		if albumKey != "\x00" {
			seenAlbums[albumKey] = true
		}
	}
	return filtered
}

func recommendationCandidateKeys(candidate database.RecommendationCandidateInput) (string, string) {
	cleanArtist, cleanAlbum, err := database.NormalizeAlbumLookup(candidate.Artist, candidate.Album)
	if err != nil {
		return normalizeStyleText(candidate.Artist), normalizeStyleText(candidate.Album)
	}
	return cleanArtist, cleanAlbum
}

func resetRecommendationCandidateRanks(candidates []database.RecommendationCandidateInput) {
	for idx := range candidates {
		candidates[idx].Rank = idx + 1
	}
}

func firstStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) == 0 {
		return nil
	}
	if len(values) < limit {
		return values
	}
	return values[:limit]
}

func discoveryCandidateInput(
	candidate mcpserver.DiscoveryCandidate,
	rank int,
	note string,
) database.RecommendationCandidateInput {
	input := database.RecommendationCandidateInput{
		Artist:       candidate.Artist,
		Album:        candidate.Album,
		StarterTrack: candidate.TrackName,
		ReleaseYear:  candidate.ReleaseYear,
		GenreTags:    candidate.GenreTags,
		Rank:         rank,
		Note:         strings.TrimSpace(note),
	}
	return input
}

func rankDiscoveryCandidatesForPrompt(
	request RecommendationRequest,
	plan modelDiscoveryPlan,
	candidates []mcpserver.DiscoveryCandidate,
) []mcpserver.DiscoveryCandidate {
	if len(candidates) < 2 {
		return candidates
	}

	ranked := append([]mcpserver.DiscoveryCandidate(nil), candidates...)
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(ranked), func(i, j int) {
		ranked[i], ranked[j] = ranked[j], ranked[i]
	})
	sort.SliceStable(ranked, func(i, j int) bool {
		leftScore := promptAlignmentScore(request, plan, ranked[i])
		rightScore := promptAlignmentScore(request, plan, ranked[j])
		return leftScore > rightScore
	})
	return ranked
}

func rankDiscoveryCandidatesForPromptSeeded(request RecommendationRequest, plan modelDiscoveryPlan, candidates []mcpserver.DiscoveryCandidate, seed int64) []mcpserver.DiscoveryCandidate {
	if len(candidates) < 2 {
		return candidates
	}
	ranked := append([]mcpserver.DiscoveryCandidate(nil), candidates...)
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(ranked), func(i, j int) { ranked[i], ranked[j] = ranked[j], ranked[i] })
	sort.SliceStable(ranked, func(i, j int) bool {
		return promptAlignmentScore(request, plan, ranked[i]) > promptAlignmentScore(request, plan, ranked[j])
	})
	return ranked
}

type recommendationFeedbackSignal struct {
	artist  string
	album   string
	genres  map[string]bool
	verdict string
	weight  int
}

func rankDiscoveryCandidatesWithFeedback(
	request RecommendationRequest,
	plan modelDiscoveryPlan,
	candidates []mcpserver.DiscoveryCandidate,
	feedback []database.RecommendationFeedbackLog,
) []mcpserver.DiscoveryCandidate {
	if len(candidates) < 2 || len(feedback) == 0 {
		return candidates
	}

	signals := make([]recommendationFeedbackSignal, 0, len(feedback))
	for _, entry := range feedback {
		artist, album := recommendationCandidateKeys(database.RecommendationCandidateInput{
			Artist: entry.Artist,
			Album:  entry.Album,
		})
		if artist == "" && album == "" {
			continue
		}
		genres := make(map[string]bool, len(entry.GenreTags))
		for _, tag := range entry.GenreTags {
			tag = normalizeStyleText(tag)
			if tag != "" {
				genres[tag] = true
			}
		}
		age, ok := recommendationFeedbackAge(entry.CreatedAt)
		if ok && age > 90*24*time.Hour {
			continue
		}
		feedbackWeight := 1
		if ok {
			feedbackWeight = recommendationFeedbackWeight(age)
		}
		signals = append(signals, recommendationFeedbackSignal{
			artist:  artist,
			album:   album,
			genres:  genres,
			verdict: entry.Verdict,
			weight:  feedbackWeight,
		})
	}
	if len(signals) == 0 {
		return candidates
	}

	ranked := append([]mcpserver.DiscoveryCandidate(nil), candidates...)
	sort.SliceStable(ranked, func(i, j int) bool {
		left := promptAlignmentScore(request, plan, ranked[i])*100 + discoveryFeedbackScore(ranked[i], signals)
		right := promptAlignmentScore(request, plan, ranked[j])*100 + discoveryFeedbackScore(ranked[j], signals)
		return left > right
	})
	return ranked
}

func discoveryFeedbackScore(candidate mcpserver.DiscoveryCandidate, signals []recommendationFeedbackSignal) int {
	artist, album := recommendationCandidateKeys(database.RecommendationCandidateInput{
		Artist: candidate.Artist,
		Album:  candidate.Album,
	})
	score := 0
	candidateGenres := make(map[string]bool, len(candidate.GenreTags))
	for _, tag := range candidate.GenreTags {
		tag = normalizeStyleText(tag)
		if tag != "" {
			candidateGenres[tag] = true
		}
	}
	for _, signal := range signals {
		weight := 0
		if signal.artist == artist {
			weight += signal.weight + 2
		}
		if signal.album == album {
			weight += signal.weight + 2
		}
		for tag := range candidateGenres {
			if signal.genres[tag] {
				score += genreFeedbackWeight(signal.verdict, signal.weight)
			}
		}
		switch signal.verdict {
		case "great", "good":
			if weight > 0 {
				score += weight
			}
		case "disliked":
			if weight > 0 {
				score -= weight
			}
		}
	}
	return score
}

func genreFeedbackWeight(verdict string, recencyWeight int) int {
	switch verdict {
	case "great":
		return recencyWeight
	case "good":
		return recencyWeight / 2
	case "disliked":
		return -recencyWeight
	default:
		return 0
	}
}

func recommendationFeedbackAge(createdAt string) (time.Duration, bool) {
	createdAt = strings.TrimSpace(createdAt)
	if createdAt == "" {
		return 0, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		created, err := time.Parse(layout, createdAt)
		if err == nil {
			age := time.Since(created)
			if age < 0 {
				age = 0
			}
			return age, true
		}
	}
	return 0, false
}

func recommendationFeedbackWeight(age time.Duration) int {
	switch {
	case age <= 7*24*time.Hour:
		return 3
	case age <= 30*24*time.Hour:
		return 2
	default:
		return 1
	}
}

func promptAlignmentScore(
	request RecommendationRequest,
	plan modelDiscoveryPlan,
	candidate mcpserver.DiscoveryCandidate,
) int {
	supported, unsupported := promptTraitCoverageForPlan(request, plan, candidate)
	score := len(supported)*6 - len(unsupported)*4
	score += comparisonAlignmentScore(comparisonSignalsForRequest(request, plan), candidate)

	for _, tag := range plan.FallbackTags {
		if candidateTagsContainAny(candidate.GenreTags, []string{tag}) {
			score += 2
		}
	}

	promptText := strings.ToLower(strings.Join([]string{request.Message, request.Mood}, " "))
	if promptRequestsVirtuosicPlaying(promptText) && styleMatchesAnyAvoidTag("technical death metal", requestAvoidTags(request)) {
		if candidateTagsContainAny(candidate.GenreTags, []string{"technical death metal", "technical death", "deathcore", "brutal death metal"}) {
			score -= 10
		}
		if candidateTagsContainAny(candidate.GenreTags, []string{"death metal"}) &&
			!candidateTagsContainAny(candidate.GenreTags, []string{"melodic death metal", "neoclassical metal", "power metal", "symphonic metal"}) {
			score -= 4
		}
	}
	return score
}

func candidateTagsContainAny(tags []string, terms []string) bool {
	for _, tag := range tags {
		for _, term := range terms {
			if styleMatchesAvoidTag(tag, term) {
				return true
			}
		}
	}
	return false
}

type comparisonSignals struct {
	ReferenceAnchors    []string
	ComparisonTraits    []string
	FalseFriendTraits   []string
	BridgeTraits        []string
	ComparisonModifiers []string
}

func comparisonSignalsForRequest(
	request RecommendationRequest,
	plan modelDiscoveryPlan,
) comparisonSignals {
	promptText := strings.ToLower(strings.Join([]string{request.Message, request.Mood}, " "))
	derived := promptDerivedComparisonPlan(promptText)
	return comparisonSignals{
		ReferenceAnchors:    mergeCompactWebStrings(plan.ReferenceAnchors, derived.ReferenceAnchors),
		ComparisonTraits:    mergeCompactWebStrings(plan.ComparisonTraits, derived.ComparisonTraits),
		FalseFriendTraits:   mergeCompactWebStrings(plan.FalseFriendTraits, derived.FalseFriendTraits),
		BridgeTraits:        mergeCompactWebStrings(plan.BridgeTraits, derived.BridgeTraits),
		ComparisonModifiers: mergeCompactWebStrings(plan.ComparisonModifiers, derived.ComparisonModifiers),
	}
}

func comparisonSignalsEmpty(signals comparisonSignals) bool {
	return len(signals.ReferenceAnchors) == 0 &&
		len(signals.ComparisonTraits) == 0 &&
		len(signals.FalseFriendTraits) == 0 &&
		len(signals.BridgeTraits) == 0 &&
		len(signals.ComparisonModifiers) == 0
}

func comparisonAlignmentScore(signals comparisonSignals, candidate mcpserver.DiscoveryCandidate) int {
	if comparisonSignalsEmpty(signals) {
		return 0
	}

	primaryMatches := supportedComparisonTraits(signals.ComparisonTraits, candidate.GenreTags)
	bridgeMatches := supportedComparisonTraits(signals.BridgeTraits, candidate.GenreTags)
	falseFriendMatches := supportedComparisonTraits(signals.FalseFriendTraits, candidate.GenreTags)

	score := len(primaryMatches)*8 + len(bridgeMatches)*4 - len(falseFriendMatches)*7
	if len(falseFriendMatches) > 0 && len(primaryMatches) == 0 && len(bridgeMatches) == 0 {
		score -= 8
	}
	if len(primaryMatches) > 0 && len(bridgeMatches) > 0 {
		score += 3
	}
	return score
}

func supportedComparisonTraits(traits []string, candidateTags []string) []string {
	supported := make([]string, 0, len(traits))
	for _, trait := range compactWebStrings(traits) {
		if comparisonTraitSupportedByTags(trait, candidateTags) {
			supported = append(supported, trait)
		}
	}
	return supported
}

func comparisonTraitSupportedByTags(trait string, candidateTags []string) bool {
	return candidateTagsContainAny(candidateTags, comparisonTraitTagTerms(trait))
}

func comparisonTraitTagTerms(trait string) []string {
	trait = normalizeStyleText(trait)
	switch {
	case strings.Contains(trait, "generic heavy metal"):
		return []string{"heavy metal", "traditional heavy metal"}
	case strings.Contains(trait, "generic death metal"):
		return []string{"death metal"}
	case strings.Contains(trait, "technical death"):
		return []string{"technical death", "technical death metal"}
	case strings.Contains(trait, "brutal death"):
		return []string{"brutal death metal"}
	case strings.Contains(trait, "deathcore"):
		return []string{"deathcore"}
	case strings.Contains(trait, "grindcore"):
		return []string{"grindcore"}
	case strings.Contains(trait, "funk metal"):
		return []string{"funk metal"}
	case strings.Contains(trait, "rap metal"):
		return []string{"rap metal"}
	case strings.Contains(trait, "alternative metal"):
		return []string{"alternative metal"}
	case strings.Contains(trait, "funk rock"):
		return []string{"funk rock"}
	case strings.Contains(trait, "jazz funk") || strings.Contains(trait, "jazz-funk"):
		return []string{"jazz-funk", "jazz funk"}
	case strings.Contains(trait, "jazz fusion") || strings.Contains(trait, "fusion"):
		return []string{"jazz fusion", "fusion", "jazz-funk"}
	case strings.Contains(trait, "electronic") || strings.Contains(trait, "synth"):
		return []string{"electronic", "electronica", "electropop", "synth-pop", "synth funk", "electro-funk"}
	case strings.Contains(trait, "groove") || strings.Contains(trait, "staccato"):
		return []string{"groove", "funk", "funk metal", "funk rock", "jazz-funk", "rap metal", "alternative metal", "dance-punk"}
	case strings.Contains(trait, "rhythm") || strings.Contains(trait, "bass"):
		return []string{"funk", "groove", "dub", "post-punk", "dance-punk", "jazz-funk", "jazz fusion", "electro-funk", "funk metal", "funk rock", "groove metal", "bass"}
	case strings.Contains(trait, "added weight") || strings.Contains(trait, "heavy"):
		return []string{"heavy", "heavy metal", "funk metal", "groove metal", "alternative metal", "sludge", "doom", "hardcore"}
	case strings.Contains(trait, "progressive"):
		return []string{"progressive metal", "progressive rock", "prog"}
	case strings.Contains(trait, "melodic death"):
		return []string{"melodic death metal"}
	case strings.Contains(trait, "melodic"):
		return []string{"melodic", "melodic metal", "power metal", "symphonic metal", "neoclassical metal"}
	case strings.Contains(trait, "atmospheric"):
		return []string{"atmospheric", "progressive rock", "post-rock", "art rock"}
	case strings.Contains(trait, "dynamic"):
		return []string{"progressive", "art rock", "post-metal"}
	case strings.Contains(trait, "neoclassical"):
		return []string{"neoclassical", "neoclassical metal", "power metal"}
	case strings.Contains(trait, "symphonic"):
		return []string{"symphonic", "symphonic metal"}
	case strings.Contains(trait, "speed metal"):
		return []string{"speed metal"}
	case strings.Contains(trait, "death metal"):
		return []string{"death metal"}
	default:
		return []string{trait}
	}
}

type promptTraitRule struct {
	name        string
	promptTerms []string
	tagTerms    []string
}

var promptTraitRules = []promptTraitRule{
	{
		name:        "virtuosic/lead-playing",
		promptTerms: []string{"virtuosic", "vituosic", "virtuoso", "shred", "shredding", "lead playing", "lead guitar", "neoclassical", "symphony x", "children of bodom"},
		tagTerms:    []string{"neoclassical", "power metal", "progressive metal", "symphonic metal", "speed metal"},
	},
	{
		name:        "melodic",
		promptTerms: []string{"melodic", "melody", "symphony x", "children of bodom"},
		tagTerms:    []string{"melodic", "power metal", "symphonic metal", "neoclassical metal"},
	},
	{
		name:        "funk/groove",
		promptTerms: []string{"funk", "funky", "groove", "groovy"},
		tagTerms:    []string{"funk", "groove", "jazz-funk", "funk metal", "funk rock", "funkcore"},
	},
	{
		name:        "bass/rhythm",
		promptTerms: []string{"bass line", "bass lines", "bassline", "low end", "rhythm section", "bass"},
		tagTerms:    []string{"funk", "groove", "dub", "post-punk", "dance-punk", "jazz-funk", "jazz fusion", "electro-funk", "funk metal", "funk rock", "groove metal", "bass"},
	},
	{
		name:        "jazz/fusion",
		promptTerms: []string{"jazz", "fusion", "knower", "brubeck", "art blakey"},
		tagTerms:    []string{"jazz", "jazz-funk", "jazz fusion", "nu jazz", "fusion", "funk"},
	},
	{
		name:        "electronic/synth",
		promptTerms: []string{"electronic", "synth", "electropop", "dubstep", "knower"},
		tagTerms:    []string{"electronic", "electronica", "electropop", "synth-pop", "synth funk", "idm", "dubstep", "electro-funk"},
	},
	{
		name:        "weird/experimental",
		promptTerms: []string{"weird", "experimental", "left-field", "left field", "avant", "strange"},
		tagTerms:    []string{"experimental", "avant", "noise", "math", "art rock", "zeuhl", "rio"},
	},
	{
		name:        "heavy",
		promptTerms: []string{"heavy", "metal", "sludge", "death", "thrash", "doom"},
		tagTerms:    []string{"metal", "heavy", "sludge", "death", "thrash", "doom", "hardcore", "grind"},
	},
}

func promptTraitCoverage(
	request RecommendationRequest,
	candidate mcpserver.DiscoveryCandidate,
) ([]string, []string) {
	return promptTraitCoverageForPlan(request, modelDiscoveryPlan{}, candidate)
}

func promptTraitCoverageForPlan(
	request RecommendationRequest,
	plan modelDiscoveryPlan,
	candidate mcpserver.DiscoveryCandidate,
) ([]string, []string) {
	promptText := strings.ToLower(strings.Join([]string{request.Message, request.Mood}, " "))
	tagText := strings.ToLower(strings.Join(candidate.GenreTags, " "))

	var supported []string
	var unsupported []string
	for _, rule := range promptTraitRules {
		if !containsAny(promptText, rule.promptTerms) {
			continue
		}
		supportedByTags := containsAny(tagText, rule.tagTerms)
		if rule.name == "virtuosic/lead-playing" {
			supportedByTags = scopedPerformanceEvidenceSupports(candidate, rule.name)
		}
		if supportedByTags {
			supported = append(supported, rule.name)
		} else {
			unsupported = append(unsupported, rule.name)
		}
	}
	signals := comparisonSignalsForRequest(request, plan)
	primaryMatches := supportedComparisonTraits(signals.ComparisonTraits, candidate.GenreTags)
	if len(primaryMatches) > 0 {
		for _, trait := range primaryMatches {
			supported = append(supported, "comparison:"+trait)
		}
	} else if len(signals.ComparisonTraits) > 0 {
		unsupported = append(unsupported, "comparison target traits")
	}
	for _, trait := range supportedComparisonTraits(signals.BridgeTraits, candidate.GenreTags) {
		supported = append(supported, "bridge:"+trait)
	}
	supported = compactWebStrings(supported)
	unsupported = compactWebStrings(unsupported)
	return supported, unsupported
}

func scopedPerformanceEvidenceSupports(candidate mcpserver.DiscoveryCandidate, trait string) bool {
	for _, evidence := range candidate.Evidence {
		if evidence.EntityScope != recommendation.EntityArtist && evidence.EntityScope != recommendation.EntityAlbum && evidence.EntityScope != recommendation.EntityRecording {
			continue
		}
		if evidence.Kind != recommendation.EvidenceCatalogIdentity && evidence.Kind != recommendation.EvidenceLocalPreference && evidence.Kind != recommendation.EvidenceLocalFit {
			continue
		}
		claim := strings.ToLower(strings.Join([]string{evidence.Claim, evidence.Details}, " "))
		if trait == "virtuosic/lead-playing" && containsAny(claim, []string{"virtuosic", "virtuoso", "shred", "lead playing", "neoclassical lead"}) {
			return true
		}
	}
	return false
}

func promptTraitLogic(request RecommendationRequest) string {
	promptText := " " + strings.ToLower(strings.Join([]string{request.Message, request.Mood}, " ")) + " "
	if strings.Contains(promptText, " or ") {
		return "any-of requested traits is acceptable because the prompt uses OR; prefer candidates that satisfy multiple traits, but do not require every trait"
	}
	return "multi-trait coverage preferred; candidates covering more requested traits should rank higher"
}

func containsAny(value string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func filterExcludedCandidates(
	candidates []database.RecommendationCandidateInput,
	exclusions database.AlbumExclusionSet,
) []database.RecommendationCandidateInput {
	if len(candidates) == 0 || exclusions.Len() == 0 {
		return candidates
	}

	filtered := make([]database.RecommendationCandidateInput, 0, len(candidates))
	for _, candidate := range candidates {
		excluded, err := exclusions.Contains(candidate.Artist, candidate.Album)
		if err != nil {
			continue
		}
		if excluded {
			continue
		}
		candidate.Rank = len(filtered) + 1
		filtered = append(filtered, candidate)
	}
	return filtered
}

func normalizeOptional(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	_, clean, err := database.NormalizeAlbumLookup("placeholder", value)
	return clean, err
}

func musicBrainzPhrase(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	return `"` + value + `"`
}

func musicBrainzReleaseGroupQuery(album string) string {
	terms := musicBrainzAlbumSearchTerms(album)
	parts := make([]string, 0, len(terms))
	for _, term := range terms {
		parts = append(parts, "releasegroup:"+musicBrainzPhrase(term))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func musicBrainzAlbumSearchTerms(album string) []string {
	seen := make(map[string]bool)
	terms := make([]string, 0, 2)
	for _, term := range []string{strings.TrimSpace(album), canonicalAlbumTitle(album)} {
		if term == "" || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	if len(terms) == 0 {
		return []string{strings.TrimSpace(album)}
	}
	return terms
}

func equivalentAlbumTitle(left string, right string) bool {
	left = canonicalAlbumTitle(left)
	right = canonicalAlbumTitle(right)
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}
	return strings.HasSuffix(left, " "+right) || strings.HasSuffix(right, " "+left)
}

func canonicalAlbumTitle(value string) string {
	_, clean, err := database.NormalizeAlbumLookup("placeholder", value)
	if err != nil {
		return ""
	}

	fields := strings.Fields(clean)
	for idx, field := range fields {
		switch field {
		case "volume":
			fields[idx] = "vol"
		}
	}
	return strings.Join(fields, " ")
}

func artistCreditName(credits []musicBrainzArtistCredit) string {
	var builder strings.Builder
	for _, credit := range credits {
		name := strings.TrimSpace(credit.Name)
		if name == "" {
			name = strings.TrimSpace(credit.Artist.Name)
		}
		builder.WriteString(name)
		builder.WriteString(credit.JoinPhrase)
	}
	return strings.TrimSpace(builder.String())
}

func isAlbumLikeReleaseGroup(primaryType string) bool {
	switch strings.ToLower(strings.TrimSpace(primaryType)) {
	case "", "album", "ep":
		return true
	default:
		return false
	}
}

func releaseYear(value string) int {
	value = strings.TrimSpace(value)
	if len(value) < 4 {
		return 0
	}
	year, err := strconv.Atoi(value[:4])
	if err != nil {
		return 0
	}
	return year
}

func releaseTrackTitles(release musicBrainzRelease) []string {
	var titles []string
	seen := make(map[string]bool)
	for _, medium := range release.Media {
		for _, track := range medium.Tracks {
			title := strings.TrimSpace(track.Title)
			if title == "" {
				title = strings.TrimSpace(track.Recording.Title)
			}
			cleanTitle, err := normalizeOptional(title)
			if title == "" || err != nil || seen[cleanTitle] {
				continue
			}
			seen[cleanTitle] = true
			titles = append(titles, title)
		}
	}
	return titles
}

func verifiedStarterTrack(suggested string, tracks []string) string {
	if len(tracks) == 0 {
		return ""
	}
	cleanSuggested, err := normalizeOptional(suggested)
	if err == nil && cleanSuggested != "" {
		for _, track := range tracks {
			cleanTrack, err := normalizeOptional(track)
			if err == nil && cleanTrack == cleanSuggested {
				return track
			}
		}
	}
	return tracks[0]
}

func topMusicBrainzTags(tags []musicBrainzTag, limit int) []string {
	if limit <= 0 || len(tags) == 0 {
		return nil
	}
	sort.SliceStable(tags, func(i, j int) bool {
		if tags[i].Count == tags[j].Count {
			return tags[i].Name < tags[j].Name
		}
		return tags[i].Count > tags[j].Count
	})

	seen := make(map[string]bool, limit)
	values := make([]string, 0, limit)
	for _, tag := range tags {
		name := strings.ToLower(strings.TrimSpace(tag.Name))
		if name == "" || seen[name] || !isUsefulMusicBrainzTag(name) {
			continue
		}
		seen[name] = true
		values = append(values, name)
		if len(values) == limit {
			break
		}
	}
	return values
}

func isUsefulMusicBrainzTag(name string) bool {
	if name == "" {
		return false
	}
	if name[0] >= '0' && name[0] <= '9' {
		return false
	}
	blockedTerms := []string{
		".com",
		".de",
		"allmusic",
		"apple music",
		"chart",
		"discogs",
		"itunes",
		"last.fm",
		"laut.de",
		"offizielle",
		"rateyourmusic",
		"spotify",
		"weeks",
		"wochen",
	}
	for _, term := range blockedTerms {
		if strings.Contains(name, term) {
			return false
		}
	}
	return true
}

func extractJSONObject(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
		return value
	}
	start := strings.Index(value, "{")
	end := strings.LastIndex(value, "}")
	if start >= 0 && end > start {
		return value[start : end+1]
	}
	return value
}

func formatNullableFloat(value sql.NullFloat64) string {
	if !value.Valid {
		return "not rated"
	}
	return fmt.Sprintf("%.2f", value.Float64)
}

func normalizeBaseURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" || strings.Contains(value, "://") {
		return value
	}
	return "http://" + value
}
