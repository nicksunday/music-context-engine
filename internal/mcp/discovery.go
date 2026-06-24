package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/nicksunday/music-context-platform/internal/utils"
)

const (
	defaultDiscoveryCandidateLimit = 5
	maxDiscoveryCandidateLimit     = 50
	maxMusicBrainzSearchLimit      = 100
	defaultDiscoveryTimeout        = 10 * time.Second
	defaultDiscoveryRequestDelay   = time.Second
	defaultMusicBrainzBaseURL      = "https://musicbrainz.org/ws/2"
	defaultDiscoveryUserAgent      = "music-context-platform/1.0.0 (https://github.com/nicksunday/music-context-platform)"
	maxDiscoveryResponseBytes      = 4 << 20
)

// DiscoveryCandidate is metadata returned by the external discovery source
// after validation and local-library exclusion.
type DiscoveryCandidate struct {
	TrackName   string `json:"track_name"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	Runtime     string `json:"runtime"`
	ReleaseYear int    `json:"release_year"`

	trackExclusionNames []string
	albumExclusionNames []string
}

type discoverySource interface {
	Search(context.Context, []string, int) ([]DiscoveryCandidate, error)
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

func getVerifiedDiscoveryCandidates(
	ctx context.Context,
	source discoverySource,
	searchTags []string,
	limit int,
	exclusions map[string]bool,
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
	if len(cleanTags) == 0 {
		return nil, fmt.Errorf("at least one discovery tag must normalize to a non-empty value")
	}

	exclusions, err = normalizeExclusionSet(exclusions)
	if err != nil {
		return nil, err
	}

	liveCandidates, err := source.Search(ctx, cleanTags, discoverySearchLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("search external discovery source: %w", err)
	}

	candidates := make([]DiscoveryCandidate, 0, limit)
	seen := make(map[string]bool)
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
		cleanTrackNames, err := normalizeDiscoveryValues(candidate.trackExclusionNames)
		if err != nil {
			return nil, fmt.Errorf("normalize candidate track names for %q: %w", candidate.TrackName, err)
		}
		if exclusions[cleanArtist] ||
			exclusions[cleanAlbum] ||
			containsExcludedValue(exclusions, cleanAlbumNames) ||
			exclusions[cleanTrack] ||
			containsExcludedValue(exclusions, cleanTrackNames) {
			continue
		}

		candidateKey := cleanArtist + "\x00" + cleanAlbum + "\x00" + cleanTrack
		if seen[candidateKey] {
			continue
		}
		seen[candidateKey] = true

		if artistCount[cleanArtist] >= 2 {
			continue
		}

		candidates = append(candidates, candidate)
		artistCount[cleanArtist]++
		if len(candidates) == limit {
			break
		}
	}

	return candidates, nil
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

func (client *musicBrainzDiscoveryClient) Search(
	ctx context.Context,
	searchTags []string,
	limit int,
) ([]DiscoveryCandidate, error) {
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

	cleanTags, err := normalizeDiscoveryTags(searchTags)
	if err != nil {
		return nil, err
	}
	if len(cleanTags) == 0 {
		return nil, fmt.Errorf("at least one MusicBrainz tag is required")
	}

	endpoint, err := client.searchEndpoint(cleanTags, limit)
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

	return parseMusicBrainzCandidates(searchResponse.Recordings), nil
}

func (client *musicBrainzDiscoveryClient) searchEndpoint(searchTags []string, limit int) (string, error) {
	endpoint, err := url.Parse(client.baseURL + "/recording")
	if err != nil {
		return "", fmt.Errorf("parse MusicBrainz base URL: %w", err)
	}

	tagClauses := make([]string, 0, len(searchTags))
	for _, tag := range searchTags {
		tagClauses = append(tagClauses, fmt.Sprintf(`tag:"%s"`, tag))
	}
	tagQuery := strings.Join(tagClauses, " OR ")
	if len(tagClauses) > 1 {
		tagQuery = "(" + tagQuery + ")"
	}

	query := endpoint.Query()
	query.Set("fmt", "json")
	query.Set("inc", "artist-rels+release-groups+aliases")
	query.Set("limit", strconv.Itoa(min(limit, maxMusicBrainzSearchLimit)))
	query.Set(
		"query",
		tagQuery+" AND primarytype:album AND status:official",
	)
	endpoint.RawQuery = query.Encode()

	return endpoint.String(), nil
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

func containsExcludedValue(exclusions map[string]bool, values []string) bool {
	for _, value := range values {
		if exclusions[value] {
			return true
		}
	}
	return false
}

func normalizeExclusionSet(exclusions map[string]bool) (map[string]bool, error) {
	normalized := make(map[string]bool, len(exclusions))
	for value, excluded := range exclusions {
		if !excluded {
			continue
		}
		cleanValue, err := utils.NormalizeSearchText(value)
		if err != nil {
			return nil, fmt.Errorf("normalize exclusion value %q: %w", value, err)
		}
		if cleanValue != "" {
			normalized[cleanValue] = true
		}
	}
	return normalized, nil
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
