package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/nicksunday/music-context-platform/internal/database"
	"github.com/nicksunday/music-context-platform/internal/recommendation"
	"github.com/nicksunday/music-context-platform/internal/utils"
)

const (
	defaultDiscoveryCandidateLimit = 5
	maxDiscoveryCandidateLimit     = 50
	maxMusicBrainzSearchLimit      = 100
	// defaultDiscoveryTimeout bounds each individual MusicBrainz HTTP call.
	// Discovery prompts are frequently expanded into many OR'd genre tags that
	// map to a single large album search, and MusicBrainz can take well over ten
	// seconds to fully expand those responses. The timeout still fails fast per
	// requests so a hung call is never left running.
	defaultDiscoveryTimeout      = 30 * time.Second
	defaultDiscoveryRequestDelay = time.Second
	defaultMusicBrainzBaseURL    = "https://musicbrainz.org/ws/2"
	defaultDiscoveryUserAgent    = "music-context-platform/1.0.0 (https://github.com/nicksunday/music-context-platform)"
	maxDiscoveryResponseBytes    = 4 << 20
	maxCandidateGenreTags        = 6
	// maxDiscoverySeedArtists bounds how many real similar-artist names are used
	// to anchor a discovery search. Each seed is a separate MusicBrainz query
	// (rate-limited), so the cap keeps per-request latency reasonable while still
	// giving the search strong artist grounding.
	maxDiscoverySeedArtists = 4
	// maxDiscoveryReconcileCandidates caps the merged artist+tag recording pool
	// fed to release-group genre reconciliation, bounding the number of
	// rate-limited release-group lookups for a single discovery request.
	maxDiscoveryReconcileCandidates = 48
)

// DiscoveryCandidate is metadata returned by the external discovery source
// after validation and local-library exclusion.
type DiscoveryCandidate struct {
	ID          string                    `json:"-"`
	TrackName   string                    `json:"track_name"`
	Artist      string                    `json:"artist"`
	Album       string                    `json:"album"`
	Runtime     string                    `json:"runtime"`
	ReleaseYear int                       `json:"release_year"`
	GenreTags   []string                  `json:"genre_tags,omitempty"`
	Evidence    []recommendation.Evidence `json:"evidence,omitempty"`

	trackExclusionNames []string
	albumExclusionNames []string
	releaseGroupID      string
}

type VerifiedDiscoveryQuery struct {
	TargetVibe   string
	FallbackTags []string
	SeedArtists  []string
	Limit        int
	// SkipAlbumGenreReconciliation avoids release-group lookups when the caller
	// only needs recording-level song candidates. Album discovery keeps the
	// default false value so its album-level genre safeguards are unchanged.
	SkipAlbumGenreReconciliation bool
}

type VerifiedDiscoveryResult struct {
	Instructions   string               `json:"instructions"`
	EffectiveLimit int                  `json:"effective_limit"`
	Candidates     []DiscoveryCandidate `json:"candidates"`
}

type discoverySource interface {
	Search(context.Context, []string, int) ([]DiscoveryCandidate, error)
}

// artistSeededDiscoverySource is optionally implemented by discovery sources
// that can anchor results on a set of (typically real, similar) artist names in
// addition to genre tags. Anchoring on artist names grounds abstract prompts in
// real compositional adjacency rather than only guessed genre tags.
type artistSeededDiscoverySource interface {
	SearchWithSeeds(context.Context, []string, []string, int) ([]DiscoveryCandidate, error)
}

type songDiscoverySource interface {
	SearchSongs(context.Context, []string, []string, int) ([]DiscoveryCandidate, error)
}

type musicBrainzDiscoveryConfig struct {
	HTTPClient   *http.Client
	BaseURL      string
	UserAgent    string
	Timeout      time.Duration
	RequestDelay time.Duration
}

type musicBrainzDiscoveryClient struct {
	httpClient   *http.Client
	baseURL      string
	userAgent    string
	timeout      time.Duration
	requestDelay time.Duration

	requestMu sync.Mutex
	lastCall  time.Time
}

type musicBrainzRecordingSearchResponse struct {
	Recordings []musicBrainzRecording `json:"recordings"`
}

type musicBrainzRecording struct {
	Title            string                    `json:"title"`
	Length           int64                     `json:"length"`
	FirstReleaseDate string                    `json:"first-release-date"`
	Aliases          []musicBrainzAlias        `json:"aliases"`
	ArtistCredit     []musicBrainzArtistCredit `json:"artist-credit"`
	Releases         []musicBrainzRelease      `json:"releases"`
	Genres           []musicBrainzTag          `json:"genres"`
	Tags             []musicBrainzTag          `json:"tags"`
}

// musicBrainzTag is a community folksonomy tag on a MusicBrainz entity. Count
// is the number of users who applied it, which we use as a relevance proxy.
type musicBrainzTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type musicBrainzAlias struct {
	Name     string `json:"name"`
	SortName string `json:"sort-name"`
	Locale   string `json:"locale"`
	Type     string `json:"type"`
	Primary  *bool  `json:"primary"`
}

type musicBrainzArtistCredit struct {
	Name       string `json:"name"`
	JoinPhrase string `json:"joinphrase"`
	Artist     struct {
		Name string `json:"name"`
	} `json:"artist"`
}

type musicBrainzRelease struct {
	Title   string                    `json:"title"`
	Status  string                    `json:"status"`
	Date    string                    `json:"date"`
	Aliases []musicBrainzAlias        `json:"aliases"`
	Group   musicBrainzReleaseGroup   `json:"release-group"`
	Media   []musicBrainzReleaseMedia `json:"media"`
}

type musicBrainzReleaseGroup struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	PrimaryType string             `json:"primary-type"`
	Aliases     []musicBrainzAlias `json:"aliases"`
}

type musicBrainzReleaseGroupLookupResponse struct {
	Genres []musicBrainzTag `json:"genres"`
	Tags   []musicBrainzTag `json:"tags"`
}

type musicBrainzReleaseMedia struct {
	Tracks []musicBrainzReleaseTrack `json:"track"`
}

type musicBrainzReleaseTrack struct {
	Title string `json:"title"`
}

func newMusicBrainzDiscoveryClient(config musicBrainzDiscoveryConfig) *musicBrainzDiscoveryClient {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = defaultMusicBrainzBaseURL
	}
	if strings.TrimSpace(config.UserAgent) == "" {
		config.UserAgent = defaultDiscoveryUserAgent
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultDiscoveryTimeout
	}
	if config.RequestDelay < 0 {
		config.RequestDelay = 0
	}

	return &musicBrainzDiscoveryClient{
		httpClient:   httpClient,
		baseURL:      strings.TrimRight(config.BaseURL, "/"),
		userAgent:    config.UserAgent,
		timeout:      config.Timeout,
		requestDelay: config.RequestDelay,
	}
}

func defaultDiscoverySource() discoverySource {
	return newMusicBrainzDiscoveryClient(musicBrainzDiscoveryConfig{
		RequestDelay: defaultDiscoveryRequestDelay,
	})
}

func GetVerifiedDiscoveryCandidates(
	ctx context.Context,
	db exclusionsDatabase,
	query VerifiedDiscoveryQuery,
) (VerifiedDiscoveryResult, error) {
	return getVerifiedDiscoveryCandidatesFromSource(ctx, db, defaultDiscoverySource(), query)
}

type exclusionsDatabase interface {
	GetDiscoveryAlbumExclusionsContext(context.Context) (database.AlbumExclusionSet, error)
}

func getVerifiedDiscoveryCandidatesFromSource(
	ctx context.Context,
	db exclusionsDatabase,
	source discoverySource,
	query VerifiedDiscoveryQuery,
) (VerifiedDiscoveryResult, error) {
	fallbackTags := compactStrings(query.FallbackTags)
	searchTags := fallbackTags
	if len(searchTags) == 0 && strings.TrimSpace(query.TargetVibe) != "" {
		searchTags = []string{query.TargetVibe}
	}
	if len(searchTags) == 0 && len(compactStrings(query.SeedArtists)) == 0 {
		return VerifiedDiscoveryResult{}, fmt.Errorf("provide a non-empty target_vibe, fallback_tags, or seed_artists")
	}
	if query.Limit <= 0 {
		return VerifiedDiscoveryResult{}, fmt.Errorf("candidate limit must be positive")
	}
	limit := clampDiscoveryCandidateLimit(query.Limit)

	exclusions, err := db.GetDiscoveryAlbumExclusionsContext(ctx)
	if err != nil {
		return VerifiedDiscoveryResult{}, fmt.Errorf("build discovery exclusion list: %w", err)
	}

	candidates, err := getVerifiedDiscoveryCandidatesSeededWithOptions(ctx, source, searchTags, compactStrings(query.SeedArtists), limit, exclusions, query.SkipAlbumGenreReconciliation)
	if err != nil {
		return VerifiedDiscoveryResult{}, err
	}

	return VerifiedDiscoveryResult{
		Instructions:   recommendationToolInstructions,
		EffectiveLimit: limit,
		Candidates:     candidates,
	}, nil
}

func getVerifiedDiscoveryCandidates(
	ctx context.Context,
	source discoverySource,
	searchTags []string,
	limit int,
	exclusions database.AlbumExclusionSet,
) ([]DiscoveryCandidate, error) {
	return getVerifiedDiscoveryCandidatesSeededWithOptions(ctx, source, searchTags, nil, limit, exclusions, false)
}

func getVerifiedDiscoveryCandidatesSeeded(
	ctx context.Context,
	source discoverySource,
	searchTags []string,
	seedArtists []string,
	limit int,
	exclusions database.AlbumExclusionSet,
) ([]DiscoveryCandidate, error) {
	return getVerifiedDiscoveryCandidatesSeededWithOptions(ctx, source, searchTags, seedArtists, limit, exclusions, false)
}

func getVerifiedDiscoveryCandidatesSeededWithOptions(
	ctx context.Context,
	source discoverySource,
	searchTags []string,
	seedArtists []string,
	limit int,
	exclusions database.AlbumExclusionSet,
	skipAlbumGenreReconciliation bool,
) ([]DiscoveryCandidate, error) {
	if source == nil {
		return nil, fmt.Errorf("discovery source is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("candidate limit must be positive")
	}
	if limit > maxDiscoveryCandidateLimit {
		limit = maxDiscoveryCandidateLimit
	}

	cleanTags, err := normalizeDiscoveryTags(searchTags)
	if err != nil {
		return nil, err
	}
	cleanSeeds := compactDiscoverySeeds(seedArtists, maxDiscoverySeedArtists)

	var liveCandidates []DiscoveryCandidate
	if skipAlbumGenreReconciliation {
		if songSource, ok := source.(songDiscoverySource); ok {
			liveCandidates, err = songSource.SearchSongs(ctx, cleanTags, cleanSeeds, discoverySearchLimit(limit))
		} else {
			liveCandidates, err = source.Search(ctx, cleanTags, discoverySearchLimit(limit))
		}
	} else if seeded, ok := source.(artistSeededDiscoverySource); ok && len(cleanSeeds) > 0 {
		liveCandidates, err = seeded.SearchWithSeeds(ctx, cleanTags, cleanSeeds, discoverySearchLimit(limit))
	} else {
		liveCandidates, err = source.Search(ctx, cleanTags, discoverySearchLimit(limit))
	}
	if err != nil {
		return nil, fmt.Errorf("search external discovery source: %w", err)
	}
	liveCandidates = diversifyDiscoveryCandidates(liveCandidates, cleanTags)
	if len(cleanTags) == 0 && len(cleanSeeds) == 0 {
		return nil, fmt.Errorf("at least one discovery tag or seed artist must be provided")
	}

	candidates := make([]DiscoveryCandidate, 0, limit)
	seen := make(map[string]bool)
	seenAlbums := make(map[string]bool)
	artistCount := make(map[string]int)
	for _, candidate := range liveCandidates {
		if err := validateDiscoveryCandidate(candidate); err != nil {
			continue
		}

		cleanArtist, err := utils.NormalizeSearchText(candidate.Artist)
		if err != nil {
			return nil, fmt.Errorf("normalize candidate artist %q: %w", candidate.Artist, err)
		}
		cleanAlbum, err := utils.NormalizeSearchText(candidate.Album)
		if err != nil {
			return nil, fmt.Errorf("normalize candidate album %q: %w", candidate.Album, err)
		}
		cleanAlbumNames, err := normalizeDiscoveryValues(candidate.albumExclusionNames)
		if err != nil {
			return nil, fmt.Errorf("normalize candidate album names for %q: %w", candidate.Album, err)
		}
		cleanTrack, err := utils.NormalizeSearchText(candidate.TrackName)
		if err != nil {
			return nil, fmt.Errorf("normalize candidate track %q: %w", candidate.TrackName, err)
		}
		if containsExcludedAlbum(exclusions, cleanArtist, cleanAlbum, cleanAlbumNames) {
			continue
		}

		candidateKey := cleanArtist + "\x00" + cleanAlbum + "\x00" + cleanTrack
		if seen[candidateKey] {
			continue
		}

		albumKeys := discoveryAlbumKeys(cleanArtist, cleanAlbum, cleanAlbumNames)
		if containsSeenValue(seenAlbums, albumKeys) {
			continue
		}

		if artistCount[cleanArtist] >= 2 {
			continue
		}

		candidates = append(candidates, candidate)
		seen[candidateKey] = true
		markSeenValues(seenAlbums, albumKeys)
		artistCount[cleanArtist]++
		if len(candidates) == limit {
			break
		}
	}

	return candidates, nil
}

func diversifyDiscoveryCandidates(candidates []DiscoveryCandidate, searchTags []string) []DiscoveryCandidate {
	return diversifyDiscoveryCandidatesWithRand(candidates, searchTags, rand.New(rand.NewSource(time.Now().UnixNano())))
}

func diversifyDiscoveryCandidatesWithRand(candidates []DiscoveryCandidate, searchTags []string, rng *rand.Rand) []DiscoveryCandidate {
	if len(candidates) < 2 {
		return candidates
	}
	hasTagEvidence := false
	for _, candidate := range candidates {
		if len(candidate.GenreTags) > 0 {
			hasTagEvidence = true
			break
		}
	}
	if !hasTagEvidence {
		return candidates
	}
	diversified := append([]DiscoveryCandidate(nil), candidates...)
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	sort.SliceStable(diversified, func(i, j int) bool {
		left := discoveryCandidateTagScore(diversified[i], searchTags)
		right := discoveryCandidateTagScore(diversified[j], searchTags)
		return left > right
	})
	for start := 0; start < len(diversified); {
		score := discoveryCandidateTagScore(diversified[start], searchTags)
		end := start + 1
		for end < len(diversified) && discoveryCandidateTagScore(diversified[end], searchTags) == score {
			end++
		}
		for idx := end - 1; idx > start; idx-- {
			swap := rng.Intn(idx-start+1) + start
			diversified[idx], diversified[swap] = diversified[swap], diversified[idx]
		}
		start = end
	}
	return diversified
}

// DiversifyDiscoveryCandidatesForEvaluation applies the production diversity
// policy with deterministic tie ordering for offline replay.
func DiversifyDiscoveryCandidatesForEvaluation(candidates []DiscoveryCandidate, searchTags []string, seed int64) []DiscoveryCandidate {
	return diversifyDiscoveryCandidatesWithRand(candidates, searchTags, rand.New(rand.NewSource(seed)))
}

func discoveryCandidateTagScore(candidate DiscoveryCandidate, searchTags []string) int {
	score := 0
	for _, candidateTag := range candidate.GenreTags {
		for _, searchTag := range searchTags {
			if strings.EqualFold(strings.TrimSpace(candidateTag), strings.TrimSpace(searchTag)) {
				score++
			}
		}
	}
	return score
}

func discoverySearchLimit(limit int) int {
	searchLimit := limit * 4
	if searchLimit < 20 {
		searchLimit = 20
	}
	if searchLimit > maxMusicBrainzSearchLimit {
		searchLimit = maxMusicBrainzSearchLimit
	}
	return searchLimit
}

func discoveryAlbumKeys(cleanArtist string, cleanAlbum string, alternateCleanAlbums []string) []string {
	values := append([]string{cleanAlbum}, alternateCleanAlbums...)
	keys := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := cleanArtist + "\x00" + value
		if seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys
}

func containsSeenValue(seen map[string]bool, values []string) bool {
	for _, value := range values {
		if seen[value] {
			return true
		}
	}
	return false
}

func markSeenValues(seen map[string]bool, values []string) {
	for _, value := range values {
		seen[value] = true
	}
}

func (client *musicBrainzDiscoveryClient) Search(
	ctx context.Context,
	searchTags []string,
	limit int,
) ([]DiscoveryCandidate, error) {
	cleanTags, err := normalizeDiscoveryTags(searchTags)
	if err != nil {
		return nil, err
	}
	if len(cleanTags) == 0 {
		return nil, fmt.Errorf("at least one MusicBrainz tag is required")
	}

	recordings, err := client.searchTagRecordings(ctx, cleanTags, limit)
	if err != nil {
		return nil, err
	}
	if len(recordings) == 0 {
		return nil, nil
	}
	candidates := parseMusicBrainzCandidates(recordings)
	return client.reconcileAlbumGenres(ctx, candidates, cleanTags), nil
}

// SearchSongs performs the same bounded recording searches as seeded album
// discovery but deliberately skips release-group genre reconciliation. The
// recording search already returns verified title/artist/release metadata, and
// song mode does not need album-level genre validation. Avoiding one lookup per
// distinct release group substantially reduces MusicBrainz load for song
// batches.
func (client *musicBrainzDiscoveryClient) SearchSongs(
	ctx context.Context,
	searchTags []string,
	seedArtists []string,
	limit int,
) ([]DiscoveryCandidate, error) {
	cleanTags, err := normalizeDiscoveryTags(searchTags)
	if err != nil {
		return nil, err
	}
	recordings, err := client.searchSeededRecordings(ctx, cleanTags, seedArtists, limit)
	if err != nil {
		return nil, err
	}
	return parseMusicBrainzCandidates(recordings), nil
}

// SearchWithSeeds anchors discovery on real similar-artist names in addition to
// genre tags. Artist-seeded results ground abstract prompts in compositional
// adjacency rather than only guessed tags; the genre-tag search is retained as a
// breadth/semantic-fallback source. Individual seed lookups are best-effort so a
// single unknown artist cannot sink the whole discovery.
func (client *musicBrainzDiscoveryClient) SearchWithSeeds(
	ctx context.Context,
	searchTags []string,
	seedArtists []string,
	limit int,
) ([]DiscoveryCandidate, error) {
	if client == nil || client.httpClient == nil {
		return nil, fmt.Errorf("MusicBrainz discovery client is not initialized")
	}

	cleanTags, _ := normalizeDiscoveryTags(searchTags)
	recordings, err := client.searchSeededRecordings(ctx, cleanTags, seedArtists, limit)
	if err != nil {
		return nil, err
	}
	if len(recordings) == 0 {
		return nil, nil
	}
	candidates := parseMusicBrainzCandidates(recordings)
	if len(cleanTags) == 0 {
		return candidates, nil
	}
	return client.reconcileAlbumGenres(ctx, candidates, cleanTags), nil
}

func (client *musicBrainzDiscoveryClient) searchSeededRecordings(
	ctx context.Context,
	cleanTags []string,
	seedArtists []string,
	limit int,
) ([]musicBrainzRecording, error) {
	seeds := compactDiscoverySeeds(seedArtists, maxDiscoverySeedArtists)
	if len(seeds) == 0 {
		return client.searchTagRecordings(ctx, cleanTags, limit)
	}
	// Search prompt tags before catalogs so they always get a discovery opportunity.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var tagRecords []musicBrainzRecording
	if len(cleanTags) > 0 {
		tagRecords, _ = client.searchTagRecordings(ctx, cleanTags, limit)
	}
	var artistRecords [][]musicBrainzRecording
	for _, artist := range seeds {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		recs, err := client.searchArtistRecordings(ctx, artist, limit)
		if err == nil {
			artistRecords = append(artistRecords, recs)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return mergeDiscoveryRecordings(tagRecords, artistRecords), nil
}

// mergeDiscoveryRecordings reserves half the pool for prompt tags, then gives
// each artist a turn. Sparse sources donate unused capacity to other sources.
func mergeDiscoveryRecordings(tags []musicBrainzRecording, artists [][]musicBrainzRecording) []musicBrainzRecording {
	seen := make(map[string]bool)
	var recordings []musicBrainzRecording
	take := func(list *[]musicBrainzRecording) bool {
		for len(*list) > 0 && len(recordings) < maxDiscoveryReconcileCandidates {
			rec := (*list)[0]
			*list = (*list)[1:]
			key := musicBrainzRecordingKey(rec)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			recordings = append(recordings, rec)
			return true
		}
		return false
	}
	for len(recordings) < (maxDiscoveryReconcileCandidates+1)/2 && take(&tags) {
	}
	// Copy slice headers so consuming lists does not mutate the caller's lists.
	artists = append([][]musicBrainzRecording(nil), artists...)
	for len(recordings) < maxDiscoveryReconcileCandidates {
		before := len(recordings)
		for i := range artists {
			take(&artists[i])
		}
		if len(recordings) == before {
			break
		}
	}
	for take(&tags) {
	}
	return recordings
}

func (client *musicBrainzDiscoveryClient) searchTagRecordings(
	ctx context.Context,
	cleanTags []string,
	limit int,
) ([]musicBrainzRecording, error) {
	cleanTags, err := normalizeDiscoveryTags(cleanTags)
	if err != nil {
		return nil, err
	}
	if len(cleanTags) == 0 {
		return nil, nil
	}
	return client.doSearch(ctx, musicBrainzTagQuery(cleanTags), limit)
}

func (client *musicBrainzDiscoveryClient) searchArtistRecordings(
	ctx context.Context,
	artist string,
	limit int,
) ([]musicBrainzRecording, error) {
	artist = strings.TrimSpace(artist)
	if artist == "" {
		return nil, nil
	}
	return client.doSearch(ctx, musicBrainzArtistQuery(artist), limit)
}

// doSearch performs a single rate-limited MusicBrainz recording search for the
// given Lucene query and returns the raw recordings.
func (client *musicBrainzDiscoveryClient) doSearch(
	ctx context.Context,
	query string,
	limit int,
) ([]musicBrainzRecording, error) {
	if client == nil || client.httpClient == nil {
		return nil, fmt.Errorf("MusicBrainz discovery client is not initialized")
	}

	requestCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()

	client.requestMu.Lock()
	defer client.requestMu.Unlock()

	if err := client.waitForRateLimit(requestCtx); err != nil {
		return nil, err
	}

	endpoint, err := client.recordingSearchEndpoint(query, limit)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create MusicBrainz request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", client.userAgent)

	response, err := client.httpClient.Do(request)
	client.lastCall = time.Now()
	if err != nil {
		return nil, fmt.Errorf("query MusicBrainz: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return nil, fmt.Errorf(
			"MusicBrainz returned HTTP %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	var searchResponse musicBrainzRecordingSearchResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxDiscoveryResponseBytes))
	if err := decoder.Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("decode MusicBrainz response: %w", err)
	}

	return searchResponse.Recordings, nil
}

func (client *musicBrainzDiscoveryClient) recordingSearchEndpoint(query string, limit int) (string, error) {
	endpoint, err := url.Parse(client.baseURL + "/recording")
	if err != nil {
		return "", fmt.Errorf("parse MusicBrainz base URL: %w", err)
	}

	params := endpoint.Query()
	params.Set("fmt", "json")
	// "artist-rels" is deliberately omitted: the search response parser only
	// consumes release-group/alias/genre/tag data, never recording-level artist
	// relationships. Asking MusicBrainz to expand unused relationships for every
	// returned recording only inflates latency on the already-heavy OR search.
	params.Set("inc", "release-groups+aliases+genres+tags")
	params.Set("limit", strconv.Itoa(min(limit, maxMusicBrainzSearchLimit)))
	params.Set("query", query)
	endpoint.RawQuery = params.Encode()

	return endpoint.String(), nil
}

// musicBrainzTagQuery builds the Lucene query over community genre tags,
// constrained to official albums.
func musicBrainzTagQuery(cleanTags []string) string {
	tagClauses := make([]string, 0, len(cleanTags))
	for _, tag := range cleanTags {
		tagClauses = append(tagClauses, fmt.Sprintf(`tag:"%s"`, tag))
	}
	tagQuery := strings.Join(tagClauses, " OR ")
	if len(tagClauses) > 1 {
		tagQuery = "(" + tagQuery + ")"
	}
	return tagQuery + " AND primarytype:album AND status:official"
}

// musicBrainzArtistQuery anchors on a specific artist so discovery can pull
// real similar-artist discographies rather than only tag-matched recordings.
func musicBrainzArtistQuery(artist string) string {
	return fmt.Sprintf(`artist:"%s" AND primarytype:album AND status:official`, artist)
}

// musicBrainzRecordingKey deduplicates recordings across multiple seed/tag
// queries by normalized title and artist credit.
func musicBrainzRecordingKey(rec musicBrainzRecording) string {
	title := strings.ToLower(strings.TrimSpace(rec.Title))
	artist := strings.ToLower(strings.TrimSpace(musicBrainzArtistName(rec.ArtistCredit)))
	if title == "" || artist == "" {
		return ""
	}
	return title + "\x00" + artist
}

// compactDiscoverySeeds trims, deduplicates, and caps the artist names used to
// anchor a discovery search, preserving the original display spelling.
func compactDiscoverySeeds(seeds []string, maxSeeds int) []string {
	if maxSeeds <= 0 {
		return nil
	}
	seen := make(map[string]bool, len(seeds))
	var out []string
	for _, raw := range seeds {
		if len(out) >= maxSeeds {
			break
		}
		clean, err := utils.NormalizeSearchText(raw)
		if err != nil || clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, strings.TrimSpace(raw))
	}
	return out
}

func normalizeDiscoveryTags(values []string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		for tag := range strings.SplitSeq(value, ",") {
			cleanTag, err := utils.NormalizeSearchText(tag)
			if err != nil {
				return nil, fmt.Errorf("normalize discovery tag %q: %w", tag, err)
			}
			if cleanTag == "" || seen[cleanTag] {
				continue
			}
			seen[cleanTag] = true
			normalized = append(normalized, cleanTag)
		}
	}
	return normalized, nil
}

func (client *musicBrainzDiscoveryClient) waitForRateLimit(ctx context.Context) error {
	if client.requestDelay <= 0 || client.lastCall.IsZero() {
		return nil
	}

	remaining := client.requestDelay - time.Since(client.lastCall)
	if remaining <= 0 {
		return nil
	}

	timer := time.NewTimer(remaining)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func parseMusicBrainzCandidates(recordings []musicBrainzRecording) []DiscoveryCandidate {
	candidates := make([]DiscoveryCandidate, 0, len(recordings))
	for _, recording := range recordings {
		release, ok := earliestOfficialAlbumRelease(recording.Releases)
		if !ok {
			continue
		}

		releaseDate := release.Date
		if releaseDate == "" {
			releaseDate = recording.FirstReleaseDate
		}
		releaseYear, err := releaseYearFromDate(releaseDate)
		if err != nil {
			continue
		}

		// MusicBrainz commonly stores transliterated tracklists as a
		// same-release-group pseudo-release instead of a direct alias.
		trackName, trackAlias := musicBrainzDisplayTitle(
			recording.Title,
			recording.Aliases,
			pseudoReleaseTrackTitles(recording.Releases, release.Group.ID),
		)
		albumAliases := append(
			append([]musicBrainzAlias(nil), release.Aliases...),
			release.Group.Aliases...,
		)
		alternateAlbumTitles := append(
			[]string{release.Group.Title},
			pseudoReleaseTitles(recording.Releases, release.Group.ID)...,
		)
		album, albumAlias := musicBrainzDisplayTitle(
			release.Title,
			albumAliases,
			alternateAlbumTitles,
		)
		candidate := DiscoveryCandidate{
			TrackName:   trackName,
			Artist:      musicBrainzArtistName(recording.ArtistCredit),
			Album:       album,
			Runtime:     runtimeFromMilliseconds(recording.Length),
			ReleaseYear: releaseYear,
			GenreTags:   musicBrainzGenreTagNames(recording.Genres, recording.Tags, maxCandidateGenreTags),
			Evidence: []recommendation.Evidence{
				{ID: "catalog_identity", Source: "musicbrainz", EntityScope: recommendation.EntityRecording, Kind: recommendation.EvidenceCatalogIdentity, Claim: "verified recording identity"},
			},
			releaseGroupID: release.Group.ID,
		}
		for index, tag := range candidate.GenreTags {
			candidate.Evidence = append(candidate.Evidence, recommendation.Evidence{ID: fmt.Sprintf("genre_proxy_%d", index+1), Source: "musicbrainz", EntityScope: recommendation.EntityAlbum, Kind: recommendation.EvidenceGenreProxy, Claim: "catalog tag", Details: tag})
		}
		if trackAlias != "" {
			candidate.trackExclusionNames = []string{recording.Title, trackAlias}
		}
		if albumAlias != "" {
			candidate.albumExclusionNames = []string{release.Title, albumAlias}
		}
		if err := validateDiscoveryCandidate(candidate); err != nil {
			continue
		}

		candidates = append(candidates, candidate)
	}
	return candidates
}

// reconcileAlbumGenres replaces recording-level genres with the genres of the
// matched release group when available. Recording tags can be stale or belong
// to a bad user classification, while recommendations are made at album level.
// A release group whose genres contradict the search is discarded so a single
// mis-tagged recording cannot smuggle an unrelated album into discovery.
func (client *musicBrainzDiscoveryClient) reconcileAlbumGenres(
	ctx context.Context,
	candidates []DiscoveryCandidate,
	searchTags []string,
) []DiscoveryCandidate {
	if len(candidates) == 0 {
		return candidates
	}

	groupGenres := make(map[string][]string)
	for _, candidate := range candidates {
		groupID := strings.TrimSpace(candidate.releaseGroupID)
		if groupID == "" {
			continue
		}
		if _, seen := groupGenres[groupID]; seen {
			continue
		}
		genres, err := client.lookupReleaseGroupGenres(ctx, groupID)
		if err != nil {
			continue
		}
		groupGenres[groupID] = genres
	}

	reconciled := make([]DiscoveryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		groupID := strings.TrimSpace(candidate.releaseGroupID)
		if genres, ok := groupGenres[groupID]; ok && len(genres) > 0 {
			if !discoveryGenresMatchSearch(genres, searchTags) {
				continue
			}
			candidate.GenreTags = genres
		}
		candidate.releaseGroupID = ""
		reconciled = append(reconciled, candidate)
	}
	return reconciled
}

func (client *musicBrainzDiscoveryClient) lookupReleaseGroupGenres(
	ctx context.Context,
	releaseGroupID string,
) ([]string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()

	if err := client.waitForRateLimit(requestCtx); err != nil {
		return nil, err
	}

	endpoint, err := url.Parse(client.baseURL + "/release-group/" + url.PathEscape(releaseGroupID))
	if err != nil {
		return nil, fmt.Errorf("parse MusicBrainz release-group URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("fmt", "json")
	query.Set("inc", "genres+tags")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create MusicBrainz release-group request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", client.userAgent)

	response, err := client.httpClient.Do(request)
	client.lastCall = time.Now()
	if err != nil {
		return nil, fmt.Errorf("query MusicBrainz release group: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return nil, fmt.Errorf(
			"MusicBrainz release group returned HTTP %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	var lookup musicBrainzReleaseGroupLookupResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxDiscoveryResponseBytes))
	if err := decoder.Decode(&lookup); err != nil {
		return nil, fmt.Errorf("decode MusicBrainz release group response: %w", err)
	}
	return musicBrainzGenreTagNames(lookup.Genres, lookup.Tags, maxCandidateGenreTags), nil
}

func discoveryGenresMatchSearch(genres []string, searchTags []string) bool {
	for _, genre := range genres {
		for _, searchTag := range searchTags {
			if discoveryGenreMatchesSearchTag(genre, searchTag) {
				return true
			}
		}
	}
	return false
}

func discoveryGenreMatchesSearchTag(genre string, searchTag string) bool {
	genre = strings.ToLower(strings.TrimSpace(genre))
	searchTag = strings.ToLower(strings.TrimSpace(searchTag))
	if genre == "" || searchTag == "" {
		return false
	}
	genreTerms := strings.Fields(genre)
	for _, term := range strings.Fields(searchTag) {
		if !containsString(genreTerms, term) {
			return false
		}
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// musicBrainzTagNames returns up to limit deduplicated tag names from tags,
// ordered by how many users applied each one (most-applied first). This is
// what lets a caller see *why* a candidate matched a tag search instead of
// just getting a bare track/artist/album triple.
func musicBrainzTagNames(tags []musicBrainzTag, limit int) []string {
	sorted := make([]musicBrainzTag, len(tags))
	copy(sorted, tags)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Count > sorted[j].Count
	})

	names := make([]string, 0, limit)
	seen := make(map[string]bool, len(sorted))
	for _, tag := range sorted {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		names = append(names, name)
		if len(names) == limit {
			break
		}
	}
	return names
}

func musicBrainzGenreTagNames(genres []musicBrainzTag, tags []musicBrainzTag, limit int) []string {
	if len(genres) > 0 {
		return musicBrainzTagNames(genres, limit)
	}
	return musicBrainzTagNames(tags, limit)
}

func musicBrainzDisplayTitle(
	title string,
	aliases []musicBrainzAlias,
	alternateTitles []string,
) (string, string) {
	title = strings.TrimSpace(title)
	alias := preferredMusicBrainzAlias(title, aliases, alternateTitles)
	if title == "" {
		return alias, alias
	}
	if alias == "" || !containsNonLatinScript(title) {
		return title, ""
	}
	if containsFold(title, alias) {
		return title, alias
	}
	return fmt.Sprintf("%s (%s)", title, alias), alias
}

func preferredMusicBrainzAlias(
	title string,
	aliases []musicBrainzAlias,
	alternateTitles []string,
) string {
	type candidate struct {
		name  string
		score int
	}

	var selected candidate
	found := false
	addCandidate := func(name string, score int) {
		name = strings.TrimSpace(name)
		if !isLatinScriptTitle(name) || strings.EqualFold(name, strings.TrimSpace(title)) {
			return
		}
		if !found || score < selected.score {
			selected = candidate{name: name, score: score}
			found = true
		}
	}

	for _, alias := range aliases {
		if strings.EqualFold(strings.TrimSpace(alias.Type), "search hint") {
			continue
		}

		name := strings.TrimSpace(alias.Name)
		if !isLatinScriptTitle(name) {
			name = strings.TrimSpace(alias.SortName)
		}

		score := 4
		if isEnglishLocale(alias.Locale) {
			score = 1
			if alias.Primary != nil && *alias.Primary {
				score = 0
			}
		} else if aliasTypeIsRomanized(alias.Type) {
			score = 2
		} else if alias.Primary != nil && *alias.Primary {
			score = 3
		}
		addCandidate(name, score)
	}
	for _, alternateTitle := range alternateTitles {
		addCandidate(alternateTitle, 5)
	}

	return selected.name
}

func pseudoReleaseTitles(releases []musicBrainzRelease, releaseGroupID string) []string {
	titles := make([]string, 0)
	for _, release := range releases {
		if !sameReleaseGroup(release.Group.ID, releaseGroupID) ||
			!strings.EqualFold(strings.TrimSpace(release.Status), "pseudo-release") {
			continue
		}
		titles = append(titles, release.Title)
	}
	return titles
}

func pseudoReleaseTrackTitles(releases []musicBrainzRelease, releaseGroupID string) []string {
	titles := make([]string, 0)
	for _, release := range releases {
		if !sameReleaseGroup(release.Group.ID, releaseGroupID) ||
			!strings.EqualFold(strings.TrimSpace(release.Status), "pseudo-release") {
			continue
		}
		for _, medium := range release.Media {
			for _, track := range medium.Tracks {
				titles = append(titles, track.Title)
			}
		}
	}
	return titles
}

func sameReleaseGroup(candidateID, selectedID string) bool {
	candidateID = strings.TrimSpace(candidateID)
	selectedID = strings.TrimSpace(selectedID)
	return candidateID != "" && selectedID != "" && candidateID == selectedID
}

func containsNonLatinScript(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) && !unicode.In(r, unicode.Latin) {
			return true
		}
	}
	return false
}

func isLatinScriptTitle(value string) bool {
	hasLatinLetter := false
	for _, r := range value {
		if !unicode.IsLetter(r) {
			continue
		}
		if !unicode.In(r, unicode.Latin) {
			return false
		}
		hasLatinLetter = true
	}
	return hasLatinLetter
}

func isEnglishLocale(locale string) bool {
	locale = strings.ToLower(strings.TrimSpace(locale))
	return locale == "en" || strings.HasPrefix(locale, "en-") || strings.HasPrefix(locale, "en_")
}

func aliasTypeIsRomanized(aliasType string) bool {
	aliasType = strings.ToLower(strings.TrimSpace(aliasType))
	return strings.Contains(aliasType, "roman") || strings.Contains(aliasType, "translit")
}

func containsFold(value, substring string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(substring))
}

func earliestOfficialAlbumRelease(releases []musicBrainzRelease) (musicBrainzRelease, bool) {
	var selected musicBrainzRelease
	found := false
	for _, release := range releases {
		if strings.TrimSpace(release.Title) == "" {
			continue
		}
		if !strings.EqualFold(release.Status, "official") {
			continue
		}
		if !strings.EqualFold(release.Group.PrimaryType, "album") {
			continue
		}
		if !found || selected.Date == "" || (release.Date != "" && release.Date < selected.Date) {
			selected = release
			found = true
		}
	}
	return selected, found
}

func musicBrainzArtistName(credits []musicBrainzArtistCredit) string {
	var artist strings.Builder
	for _, credit := range credits {
		name := strings.TrimSpace(credit.Name)
		if name == "" {
			name = strings.TrimSpace(credit.Artist.Name)
		}
		if name == "" {
			continue
		}
		artist.WriteString(name)
		artist.WriteString(credit.JoinPhrase)
	}
	return strings.TrimSpace(artist.String())
}

func releaseYearFromDate(date string) (int, error) {
	date = strings.TrimSpace(date)
	if len(date) < 4 {
		return 0, fmt.Errorf("release date %q has no four-digit year", date)
	}
	year, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0, fmt.Errorf("parse release year from %q: %w", date, err)
	}
	return year, nil
}

func runtimeFromMilliseconds(milliseconds int64) string {
	if milliseconds <= 0 {
		return ""
	}
	totalSeconds := int64(math.Round(float64(milliseconds) / float64(time.Second/time.Millisecond)))
	if totalSeconds <= 0 {
		return ""
	}
	return fmt.Sprintf("%d:%02d", totalSeconds/60, totalSeconds%60)
}

func normalizeDiscoveryValues(values []string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		cleanValue, err := utils.NormalizeSearchText(value)
		if err != nil {
			return nil, err
		}
		if cleanValue != "" {
			normalized = append(normalized, cleanValue)
		}
	}
	return normalized, nil
}

func containsExcludedAlbum(
	exclusions database.AlbumExclusionSet,
	cleanArtist string,
	cleanAlbum string,
	alternateCleanAlbums []string,
) bool {
	if exclusions.ContainsNormalized(cleanArtist, cleanAlbum) {
		return true
	}
	for _, value := range alternateCleanAlbums {
		if exclusions.ContainsNormalized(cleanArtist, value) {
			return true
		}
	}
	return false
}

func validateDiscoveryCandidate(candidate DiscoveryCandidate) error {
	for field, value := range map[string]string{
		"track_name": candidate.TrackName,
		"artist":     candidate.Artist,
		"album":      candidate.Album,
	} {
		cleanValue, err := utils.NormalizeSearchText(value)
		if err != nil {
			return fmt.Errorf("normalize %s: %w", field, err)
		}
		if cleanValue == "" {
			return fmt.Errorf("%s is empty", field)
		}
	}

	runtimeParts := strings.Split(candidate.Runtime, ":")
	if len(runtimeParts) != 2 {
		return fmt.Errorf("runtime %q must use M:SS format", candidate.Runtime)
	}
	minutes, minuteErr := strconv.Atoi(runtimeParts[0])
	seconds, secondErr := strconv.Atoi(runtimeParts[1])
	if minuteErr != nil || secondErr != nil || minutes < 0 || seconds < 0 || seconds > 59 || len(runtimeParts[1]) != 2 {
		return fmt.Errorf("runtime %q must use M:SS format", candidate.Runtime)
	}
	if minutes == 0 && seconds == 0 {
		return fmt.Errorf("runtime must be greater than zero")
	}
	if candidate.ReleaseYear < 1900 || candidate.ReleaseYear > time.Now().Year() {
		return fmt.Errorf("release year %d is outside the supported range", candidate.ReleaseYear)
	}

	return nil
}
