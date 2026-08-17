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
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nicksunday/music-context-platform/internal/database"
	mcpserver "github.com/nicksunday/music-context-platform/internal/mcp"
)

const (
	defaultArtistLimit         = 12
	defaultGenreLimit          = 20
	defaultFeedbackLimit       = 25
	defaultBatchLimit          = 6
	maxBatchLimit              = 10
	defaultOllamaTimeout       = 3 * time.Minute
	defaultVerifyTimeout       = 10 * time.Second
	defaultVerifyDelay         = time.Second
	defaultRadarTimeout        = 25 * time.Second
	defaultRadarArtists        = 50
	defaultRadarReleases       = 8
	defaultRadarCacheTTL       = 12 * time.Hour
	defaultRadarErrorTTL       = 5 * time.Minute
	defaultRadarRefreshTimeout = 3 * time.Minute
	listenBrainzResponseLimit  = 32 << 20
	newReleaseWindowDays       = 90
	listenBrainzBaseURL        = "https://api.listenbrainz.org/1"
	musicBrainzBaseURL         = "https://musicbrainz.org/ws/2"
	iTunesSearchBaseURL        = "https://itunes.apple.com/search"
	webUserAgent               = "music-context-platform/1.0.0 (https://github.com/nicksunday/music-context-platform)"
)

var errReleaseRadarRefreshInProgress = errors.New("release radar refresh is in progress")

//go:embed static/*
var staticFiles embed.FS

type Options struct {
	Recommender  Recommender
	Verifier     CandidateVerifier
	ReleaseRadar ReleaseRadarProvider
	Model        string
	OllamaURL    string
	Timeout      time.Duration
}

type Server struct {
	db           *sql.DB
	recommender  Recommender
	verifier     CandidateVerifier
	releaseRadar ReleaseRadarProvider
	model        string
	ollamaURL    string
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

type ReleaseRadarProvider interface {
	NewReleases(context.Context, []database.ArtistAffinity, time.Time, time.Time, int) ([]NewRelease, error)
}

type DiscoveryRequest struct {
	TargetVibe   string
	FallbackTags []string
	Limit        int
}

type RecommendationRequest struct {
	Message string `json:"message"`
	Mood    string `json:"mood,omitempty"`
	Avoid   string `json:"avoid,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type RecommendationDraft struct {
	Reply      string                                  `json:"reply"`
	Candidates []database.RecommendationCandidateInput `json:"candidates"`
}

type ProfileContext struct {
	Artists        []database.ArtistAffinity            `json:"artists"`
	Genres         []database.GenreTopography           `json:"genres"`
	RecentFeedback []database.RecommendationFeedbackLog `json:"recent_feedback"`
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
	Reply string                       `json:"reply"`
	Batch database.RecommendationBatch `json:"batch"`
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
		db:           db,
		recommender:  options.Recommender,
		verifier:     options.Verifier,
		releaseRadar: options.ReleaseRadar,
		model:        strings.TrimSpace(options.Model),
		ollamaURL:    normalizeBaseURL(options.OllamaURL),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/context", server.handleContext)
	mux.HandleFunc("GET /api/rail", server.handleRail)
	mux.HandleFunc("POST /api/recommendations", server.handleRecommendations)
	mux.HandleFunc("POST /api/feedback", server.handleFeedback)
	mux.Handle("/", staticHandler())
	return mux
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
	}
}

func staticHandler() http.Handler {
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(staticRoot))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" {
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
	input.Limit = clampBatchLimit(input.Limit)
	if input.Message == "" {
		writeJSONError(writer, http.StatusBadRequest, "Please provide a recommendation prompt.")
		return
	}

	profile, err := server.fetchProfileContext(request.Context())
	if err != nil {
		writeJSONError(writer, http.StatusInternalServerError, "Unable to load music profile context.")
		return
	}
	exclusions, err := (&database.DB{Ctx: server.db}).GetExclusionListContext(request.Context())
	if err != nil {
		writeJSONError(writer, http.StatusInternalServerError, "Unable to load local discovery exclusions.")
		return
	}

	draft, err := server.recommender.Recommend(request.Context(), input, profile)
	if err != nil {
		writeJSONError(writer, http.StatusBadGateway, err.Error())
		return
	}
	if len(draft.Candidates) == 0 {
		writeJSONError(writer, http.StatusBadGateway, "The recommendation model returned no album candidates.")
		return
	}
	draft.Candidates = filterExcludedCandidates(draft.Candidates, exclusions)
	if len(draft.Candidates) == 0 {
		writeJSONError(writer, http.StatusBadGateway, "The recommendation model only returned artists or albums already in the local library or recent feedback. Try again with a more specific prompt.")
		return
	}
	draft.Candidates, err = server.verifyCandidates(request.Context(), draft.Candidates)
	if err != nil {
		writeJSONError(writer, http.StatusBadGateway, err.Error())
		return
	}
	if len(draft.Candidates) == 0 {
		writeJSONError(writer, http.StatusBadGateway, "No generated album candidates survived external verification. Try again with a more specific prompt.")
		return
	}
	if len(draft.Candidates) > input.Limit {
		draft.Candidates = draft.Candidates[:input.Limit]
	}

	batch, err := database.CreateRecommendationBatch(request.Context(), server.db, database.RecommendationBatchInput{
		Prompt:     input.Message,
		Mood:       input.Mood,
		Notes:      draft.Reply,
		Candidates: draft.Candidates,
	})
	if err != nil {
		writeJSONError(writer, http.StatusInternalServerError, "Unable to persist recommendation batch.")
		return
	}

	writeJSON(writer, http.StatusOK, RecommendationResponse{
		Reply: strings.TrimSpace(draft.Reply),
		Batch: batch,
	})
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
}

type modelDiscoveryPlan struct {
	VibeSummary    string   `json:"vibe_summary"`
	RequiredTraits []string `json:"required_traits"`
	FlexibleTraits []string `json:"flexible_traits"`
	TargetVibe     string   `json:"target_vibe"`
	FallbackTags   []string `json:"fallback_tags"`
}

type modelCandidateSelection struct {
	CandidateIndex int    `json:"candidate_index"`
	Note           string `json:"note"`
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

type iTunesSearchResponse struct {
	Results []iTunesSearchResult `json:"results"`
}

type iTunesSearchResult struct {
	ArtistName       string `json:"artistName"`
	CollectionName   string `json:"collectionName"`
	PrimaryGenreName string `json:"primaryGenreName"`
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
	candidates := make([]NewRelease, 0, limit)
	seen := make(map[string]bool)
	for _, release := range payload.Payload.Releases {
		if len(candidates) == limit {
			break
		}
		if !strings.EqualFold(strings.TrimSpace(release.ReleaseGroupPrimaryType), "album") {
			continue
		}
		releaseDate := strings.TrimSpace(release.ReleaseDate)
		releaseTime, ok := parseMusicBrainzDate(releaseDate)
		if !ok || releaseTime.Before(dayStart(since)) || releaseTime.After(dayStart(until)) {
			continue
		}
		cleanArtist, cleanAlbum, err := database.NormalizeAlbumLookup(release.ArtistCreditName, release.ReleaseName)
		if err != nil || cleanArtist == "" || cleanAlbum == "" || !artistSet[cleanArtist] {
			continue
		}
		key := cleanArtist + "\x00" + cleanAlbum
		if seen[key] {
			continue
		}
		seen[key] = true

		genreTags := cleanReleaseTags(release.ReleaseTags, 4)
		if len(genreTags) == 0 {
			genreTags, _ = radar.releaseGroupTags(ctx, release.ReleaseGroupMBID)
		}
		if len(genreTags) == 0 {
			genreTags, _ = radar.iTunesAlbumTags(ctx, release.ArtistCreditName, release.ReleaseName)
		}
		candidates = append(candidates, NewRelease{
			Artist:      strings.TrimSpace(release.ArtistCreditName),
			Album:       strings.TrimSpace(release.ReleaseName),
			ReleaseDate: releaseDate,
			ReleaseType: release.ReleaseGroupPrimaryType,
			GenreTags:   genreTags,
			Blurb:       releaseBlurb(genreTags),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return releaseDateSortKey(candidates[i].ReleaseDate) > releaseDateSortKey(candidates[j].ReleaseDate)
	})
	return candidates, nil
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
		TargetVibe:   request.TargetVibe,
		FallbackTags: request.FallbackTags,
		Limit:        request.Limit,
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

	plan, err := recommender.planDiscovery(ctx, request, profile)
	if err != nil {
		return RecommendationDraft{}, err
	}
	if len(plan.FallbackTags) == 0 && strings.TrimSpace(plan.TargetVibe) == "" {
		plan.TargetVibe = request.Message
	}

	candidates, err := recommender.discovery.Discover(ctx, DiscoveryRequest{
		TargetVibe:   plan.TargetVibe,
		FallbackTags: compactWebStrings(plan.FallbackTags),
		Limit:        discoveryCandidateFetchLimit(request.Limit),
	})
	if err != nil {
		return RecommendationDraft{}, fmt.Errorf("MCP verified discovery failed: %w", err)
	}
	if len(candidates) == 0 {
		return RecommendationDraft{}, errors.New("MCP verified discovery returned no candidates for those tags. Try a slightly broader prompt.")
	}

	return recommender.selectDiscoveryCandidates(ctx, request, profile, plan, candidates)
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
	plan.TargetVibe = strings.TrimSpace(plan.TargetVibe)
	plan.FallbackTags = compactWebStrings(plan.FallbackTags)
	plan = calibrateDiscoveryPlanForPrompt(request, plan)
	return plan, nil
}

func (recommender *MCPGroundedOllamaRecommender) selectDiscoveryCandidates(
	ctx context.Context,
	request RecommendationRequest,
	profile ProfileContext,
	plan modelDiscoveryPlan,
	candidates []mcpserver.DiscoveryCandidate,
) (RecommendationDraft, error) {
	limit := clampBatchLimit(request.Limit)
	content, err := recommender.ollama.chat(
		ctx,
		discoverySelectionSystemPrompt(),
		discoverySelectionUserPrompt(request, profile, plan, candidates, limit),
	)
	if err != nil {
		return fallbackDiscoveryDraft(candidates, plan, limit), nil
	}

	var selection modelCandidateSelectionResponse
	if err := json.Unmarshal([]byte(extractJSONObject(content)), &selection); err != nil {
		return fallbackDiscoveryDraft(candidates, plan, limit), nil
	}

	draft := RecommendationDraft{Reply: strings.TrimSpace(selection.Reply)}
	seen := make(map[int]bool)
	for _, selected := range selection.Selections {
		idx := selected.CandidateIndex - 1
		if idx < 0 || idx >= len(candidates) || seen[idx] {
			continue
		}
		seen[idx] = true
		draft.Candidates = append(draft.Candidates, discoveryCandidateInput(
			candidates[idx],
			len(draft.Candidates)+1,
			selected.Note,
		))
		if len(draft.Candidates) == limit {
			break
		}
	}
	if len(draft.Candidates) == 0 {
		return fallbackDiscoveryDraft(candidates, plan, limit), nil
	}
	if draft.Reply == "" {
		draft.Reply = "Here are verified albums from the MCP discovery search."
	}
	return draft, nil
}

func discoveryPlanSystemPrompt() string {
	return `You interpret personal music recommendation prompts into a musical discovery plan, then map that plan to canonical MusicBrainz genre/tag searches.
Return only JSON with this exact shape:
{"vibe_summary":"short musical intent","required_traits":["trait"],"flexible_traits":["trait"],"target_vibe":"","fallback_tags":["tag one","tag two"]}

Rules:
- Treat the user's words as evidence of the intended listening feel, not a literal checklist, unless they use explicit hard constraints like "must", "only", "no", or "avoid".
- Infer musical intent across energy, rhythm feel, texture, density, vocal/lyric importance, novelty, and mood before choosing tags.
- Put true hard requirements in required_traits. Put vibe cues, loose descriptors, and "or" alternatives in flexible_traits.
- For prompts like "heavy or funky or both", do not require every candidate to be both heavy and funky; search broadly enough to find strong candidates from either side, while preferring overlap.
- Prefer fallback_tags with 3 to 8 lower-case genre, style, or scene tags likely to exist in MusicBrainz.
- Use target_vibe only when the prompt is already one canonical tag.
- Active prompt ingredients outrank the user's historical genre profile. Use profile context only for calibration, never to erase requested traits.
- Do not add metal/heavy tags merely because the user's profile is metal-heavy. Only include metal/heavy tags when the current prompt asks for metal, heavy, sludge, death, thrash, doom, grind, or adjacent weight.
- For cross-genre prompts, include each important vibe ingredient as separate tags instead of collapsing everything into one phrase.
- When the prompt references non-metal artists/styles such as KNOWER, jazz-funk, funk, fusion, electronic, country, bluegrass, or dubstep, search those spaces directly instead of translating them into metal-adjacent tags.
- When the prompt says "funk", "funky", or "groove", include tags such as "funk", "jazz-funk", "funk rock", "electro-funk", or "dance-punk"; use "funk metal" only if metal/heavy is also requested.
- When the prompt asks for "bass lines", "bassline", "low end", or rhythm-section feel, use genre proxies such as "funk", "jazz-funk", "dub", "post-punk", "dance-punk", "electro-funk", or "jazz fusion"; MusicBrainz tags rarely encode bass performance directly.
- When the prompt names KNOWER or similar jazz/electronic fusion acts, include tags such as "jazz-funk", "jazz fusion", "electropop", "synth-pop", and "funk".
- When the prompt says "weird", "experimental", or "left-field", include tags such as "avant-garde metal", "experimental rock", "noise rock", "math rock", or "art rock" when musically compatible.
- When the prompt says "heavy", include specific heavy tags that preserve any other requested ingredients, such as "funk metal", "alternative metal", "progressive metal", "death metal", or "sludge metal".
- Do not output artist names, album names, vague adjectives, moods, or prose in fallback_tags.
- Avoid tags that recent feedback says are not aligned with the user.`
}

func discoverySelectionSystemPrompt() string {
	return `You rank verified MCP discovery candidates for a personal music recommendation batch.
Return only JSON with this exact shape:
{"reply":"short plain-language summary","selections":[{"candidate_index":1,"note":"one concise reason"}]}

Rules:
- candidate_index must be one of the numbered candidates provided by the user message.
- Do not create, rename, or substitute artists, albums, or tracks.
- Recommend albums; the listed track is the verified MCP matched track that brought the album into the candidate set.
- Present the listed track as a matched entry point, not necessarily the album's best opener or most representative song.
- Use vibe_summary, required_traits, and flexible_traits as the main intent. Treat MusicBrainz tags as evidence, not the full meaning of the prompt.
- Prefer high-impact matches to the user's current prompt over generic taste-anchor similarity.
- Treat taste anchors as guardrails and quality calibration, not as target genres. Do not pull the batch toward metal unless the current prompt asks for metal/heavy.
- Preserve non-metal request terms such as jazz, funk, groove, electronic, country, dubstep, weird, or experimental even when the user's profile has strong metal affinities.
- Prefer candidates whose genre_tags cover multiple prompt ingredients over candidates that only match the broadest or most familiar tag.
- Use supported_prompt_traits and unsupported_prompt_traits exactly as provided. Do not claim an unsupported trait in a note.
- If the prompt asks for funk/funky/groove, rank candidates with supported_prompt_traits containing funk/groove above candidates where that trait is unsupported.
- If prompt_trait_logic says any-of, candidates may satisfy either side of an "or" request; describe only their supported traits.
- Never mention unsupported_prompt_traits, unsupported traits, or support-status bookkeeping in the user-facing reply or notes.
- Keep each note to one short sentence and do not invent sourcing, reviews, credits, or track details.`
}

func recommendationSystemPrompt() string {
	return `You generate album-first music recommendations for a local personal library.
Return only JSON with this exact shape:
{"reply":"short plain-language summary","candidates":[{"artist":"Artist","album":"Album","starter_track":"Track to sample","release_year":2024,"genre_tags":["tag"],"note":"one concise reason"}]}

Rules:
- Recommend albums, not playlists and not individual single-only tracks.
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
	builder.WriteString("Planning priority: infer the intended musical feel before choosing tags. The user's current prompt owns the search space; profile data calibrates quality and novelty but must not drag every prompt back to the user's top genres. Do not treat casual wording as a literal checklist unless the user states a hard constraint. If the prompt asks for non-metal jazz, funk, bass-forward, electronic, country, bluegrass, or dubstep, search those spaces directly. If it asks for new music with great bass lines that is heavy or funky or both, infer a search for prominent low-end/rhythm-section energy, groove, and weight; heavy-only, funky-only, and overlapping candidates may all be valid. If it asks for bass lines, low end, or rhythm-section feel, include groove/bass proxy tags such as funk, jazz-funk, dub, post-punk, dance-punk, electro-funk, or jazz fusion. If it contains weird/experimental, include an experimental or avant-garde tag.\n")

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
	fmt.Fprintf(&builder, "Prompt trait logic: %s\n", promptTraitLogic(request))
	if plan.VibeSummary != "" {
		fmt.Fprintf(&builder, "Interpreted vibe: %s\n", plan.VibeSummary)
	}
	if len(plan.RequiredTraits) > 0 {
		fmt.Fprintf(&builder, "Required traits: %s\n", strings.Join(plan.RequiredTraits, ", "))
	}
	if len(plan.FlexibleTraits) > 0 {
		fmt.Fprintf(&builder, "Flexible traits: %s\n", strings.Join(plan.FlexibleTraits, ", "))
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
		supported, unsupported := promptTraitCoverage(request, candidate)
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

func decodeJSON(request *http.Request, target any) error {
	defer request.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeJSONError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}

func clampBatchLimit(limit int) int {
	if limit <= 0 {
		return defaultBatchLimit
	}
	if limit > maxBatchLimit {
		return maxBatchLimit
	}
	return limit
}

func discoveryCandidateFetchLimit(limit int) int {
	limit = clampBatchLimit(limit) * 3
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

	tags := compactWebStrings(plan.FallbackTags)
	if promptRequestsHeavyMusic(promptText) {
		if len(tags) == 0 {
			tags = prependDiscoveryTags(promptDerivedDiscoveryTags(promptText), nil, 8)
		}
	} else {
		tags = filterDiscoveryTags(tags, func(tag string) bool {
			return !isMetalDiscoveryTag(tag)
		})
		tags = prependDiscoveryTags(promptDerivedDiscoveryTags(promptText), tags, 8)
	}
	plan.FallbackTags = tags
	return plan
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
	}
	return artistSet
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
		if len(draft.Candidates) == limit {
			break
		}
		note := "Verified by MCP discovery"
		if len(plan.FallbackTags) > 0 {
			note = "Returned by MCP discovery for " + strings.Join(plan.FallbackTags, ", ") + "."
		} else if plan.TargetVibe != "" {
			note = "Returned by MCP discovery for " + plan.TargetVibe + "."
		}
		draft.Candidates = append(draft.Candidates, discoveryCandidateInput(candidate, idx+1, note))
	}
	return draft
}

func discoveryCandidateInput(
	candidate mcpserver.DiscoveryCandidate,
	rank int,
	note string,
) database.RecommendationCandidateInput {
	return database.RecommendationCandidateInput{
		Artist:       candidate.Artist,
		Album:        candidate.Album,
		StarterTrack: candidate.TrackName,
		ReleaseYear:  candidate.ReleaseYear,
		GenreTags:    candidate.GenreTags,
		Rank:         rank,
		Note:         strings.TrimSpace(note),
	}
}

type promptTraitRule struct {
	name        string
	promptTerms []string
	tagTerms    []string
}

var promptTraitRules = []promptTraitRule{
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
	promptText := strings.ToLower(strings.Join([]string{request.Message, request.Mood}, " "))
	tagText := strings.ToLower(strings.Join(candidate.GenreTags, " "))

	var supported []string
	var unsupported []string
	for _, rule := range promptTraitRules {
		if !containsAny(promptText, rule.promptTerms) {
			continue
		}
		if containsAny(tagText, rule.tagTerms) {
			supported = append(supported, rule.name)
		} else {
			unsupported = append(unsupported, rule.name)
		}
	}
	return supported, unsupported
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
	exclusions map[string]bool,
) []database.RecommendationCandidateInput {
	if len(candidates) == 0 || len(exclusions) == 0 {
		return candidates
	}

	filtered := make([]database.RecommendationCandidateInput, 0, len(candidates))
	for _, candidate := range candidates {
		cleanArtist, cleanTitle, err := database.NormalizeAlbumLookup(candidate.Artist, candidate.Album)
		if err != nil {
			continue
		}
		cleanTrack, err := normalizeOptional(candidate.StarterTrack)
		if err != nil {
			continue
		}
		if exclusions[cleanArtist] || exclusions[cleanTitle] || (cleanTrack != "" && exclusions[cleanTrack]) {
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
