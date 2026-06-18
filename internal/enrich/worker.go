package enrich

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nicksunday/music-context-platform/internal/utils"
)

const (
	LastFMAPIKeyEnv = "LASTFM_API_KEY"

	DefaultMusicBrainzBaseURL = "https://musicbrainz.org/ws/2"
	DefaultLastFMBaseURL      = "https://ws.audioscrobbler.com/2.0/"
	DefaultRequestDelay       = time.Second
	DefaultMaxGenres          = 5
	DefaultUserAgent          = "music-context-platform/1.0.0 (https://github.com/nicksunday/music-context-platform)"
	lastFMArtistNotFoundCode  = 6
)

var lastFMGenreWhitelist = []string{
	"metal",
	"core",
	"prog",
	"sludge",
	"death",
	"djent",
	"avant-garde",
	"avant garde",
	"experimental",
	"thrash",
	"technical",
	"instrumental",
}

var errArtistNotFound = errors.New("artist not found externally")

type Config struct {
	HTTPClient         *http.Client
	Logger             *log.Logger
	MusicBrainzBaseURL string
	LastFMBaseURL      string
	LastFMAPIKey       string
	UserAgent          string
	RequestDelay       time.Duration
	DisableRateLimit   bool
	MaxGenres          int
}

type Worker struct {
	db        *sql.DB
	http      *http.Client
	config    Config
	requestMu sync.Mutex
	lastCall  time.Time
}

type RunResult struct {
	AlbumsScanned  int
	AlbumsUpdated  int
	ArtistsScanned int
	ArtistsUpdated int
	RecordsUpdated int64
}

type albumRecord struct {
	id                string
	title             string
	artist            string
	cleanTitle        string
	cleanArtist       string
	genresMissing     bool
	trackCountMissing bool
}

type artistRecord struct {
	name        string
	cleanArtist string
}

type albumMetadata struct {
	genres     []string
	trackCount sql.NullInt64
}

type genreCandidate struct {
	name  string
	count int
	index int
}

type musicBrainzSearchResponse struct {
	Artists []struct {
		ID string `json:"id"`
	} `json:"artists"`
}

type musicBrainzArtistResponse struct {
	Genres []apiTag `json:"genres"`
	Tags   []apiTag `json:"tags"`
}

type musicBrainzReleaseSearchResponse struct {
	Releases []struct {
		ID string `json:"id"`
	} `json:"releases"`
}

type musicBrainzReleaseResponse struct {
	Genres []apiTag            `json:"genres"`
	Tags   []apiTag            `json:"tags"`
	Media  []musicBrainzMedium `json:"media"`
}

type musicBrainzMedium struct {
	TrackCount int               `json:"track-count"`
	Tracks     []json.RawMessage `json:"tracks"`
}

type apiTag struct {
	Name  string        `json:"name"`
	Count flexibleCount `json:"count"`
}

type lastFMTopTagsResponse struct {
	Error   int    `json:"error"`
	Message string `json:"message"`
	TopTags struct {
		Tags []apiTag `json:"tag"`
	} `json:"toptags"`
}

type flexibleCount int

func DefaultConfig() Config {
	return Config{
		MusicBrainzBaseURL: DefaultMusicBrainzBaseURL,
		LastFMBaseURL:      DefaultLastFMBaseURL,
		UserAgent:          DefaultUserAgent,
		RequestDelay:       DefaultRequestDelay,
		MaxGenres:          DefaultMaxGenres,
	}
}

func NewWorker(db *sql.DB, config Config) *Worker {
	config = withDefaults(config)
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Worker{
		db:     db,
		http:   httpClient,
		config: config,
	}
}

func (w *Worker) Run(ctx context.Context) (RunResult, error) {
	var result RunResult

	albums, err := w.albumsNeedingMetadata(ctx)
	if err != nil {
		return result, err
	}
	result.AlbumsScanned = len(albums)

	for _, album := range albums {
		metadata, err := w.fetchAlbumMetadata(ctx, album)
		if err != nil {
			if errors.Is(err, errArtistNotFound) {
				w.warnf("Album %s by %s not found externally. Skipping.", album.title, album.artist)
				metadata = albumMetadata{genres: []string{}}
			} else {
				return result, fmt.Errorf("failed to enrich album %q by %q: %w", album.title, album.artist, err)
			}
		}

		recordsUpdated, err := w.commitAlbumMetadata(ctx, album, metadata)
		if err != nil {
			return result, fmt.Errorf("failed to commit metadata for album %q by %q: %w", album.title, album.artist, err)
		}
		if recordsUpdated > 0 {
			result.AlbumsUpdated++
			result.RecordsUpdated += recordsUpdated
			w.logf("enriched album %q by %q with genres %v and track_count %s", album.title, album.artist, metadata.genres, formatNullableTrackCount(metadata.trackCount))
		}
	}

	inheritedTracks, err := w.inheritTrackGenresFromAlbums(ctx)
	if err != nil {
		return result, err
	}
	result.RecordsUpdated += inheritedTracks

	artists, err := w.artistsNeedingGenres(ctx)
	if err != nil {
		return result, err
	}
	result.ArtistsScanned = len(artists)

	for _, artist := range artists {
		genres, err := w.FetchArtistGenres(ctx, artist.name)
		if err != nil {
			if errors.Is(err, errArtistNotFound) {
				w.warnf("Artist %s not found externally. Skipping.", artist.name)

				recordsUpdated, commitErr := w.commitTrackGenres(ctx, artist, []string{})
				if commitErr != nil {
					return result, fmt.Errorf("failed to commit placeholder genres for %q: %w", artist.name, commitErr)
				}
				result.RecordsUpdated += recordsUpdated
				continue
			}
			return result, fmt.Errorf("failed to enrich artist %q: %w", artist.name, err)
		}
		if len(genres) == 0 {
			w.warnf("Artist %s not found externally. Skipping.", artist.name)

			recordsUpdated, err := w.commitTrackGenres(ctx, artist, []string{})
			if err != nil {
				return result, fmt.Errorf("failed to commit placeholder genres for %q: %w", artist.name, err)
			}
			result.RecordsUpdated += recordsUpdated
			continue
		}

		recordsUpdated, err := w.commitTrackGenres(ctx, artist, genres)
		if err != nil {
			return result, fmt.Errorf("failed to commit genres for %q: %w", artist.name, err)
		}
		if recordsUpdated > 0 {
			result.ArtistsUpdated++
			result.RecordsUpdated += recordsUpdated
			w.logf("enriched %q with %v", artist.name, genres)
		}
	}

	return result, nil
}

func (w *Worker) albumsNeedingMetadata(ctx context.Context) ([]albumRecord, error) {
	const stmt = `
		SELECT
			id,
			title,
			artist,
			clean_title,
			clean_artist,
			genres IS NULL AS genres_missing,
			track_count IS NULL AS track_count_missing
		FROM albums
		WHERE (genres IS NULL OR track_count IS NULL)
			AND TRIM(title) != ''
			AND TRIM(artist) != ''
		ORDER BY artist COLLATE NOCASE, title COLLATE NOCASE`

	rows, err := w.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("failed to query albums needing metadata: %w", err)
	}
	defer rows.Close()

	var albums []albumRecord
	for rows.Next() {
		var (
			album             albumRecord
			genresMissing     int
			trackCountMissing int
		)
		if err := rows.Scan(
			&album.id,
			&album.title,
			&album.artist,
			&album.cleanTitle,
			&album.cleanArtist,
			&genresMissing,
			&trackCountMissing,
		); err != nil {
			return nil, err
		}

		album.title = strings.TrimSpace(album.title)
		album.artist = strings.TrimSpace(album.artist)
		album.cleanTitle = strings.TrimSpace(album.cleanTitle)
		album.cleanArtist = strings.TrimSpace(album.cleanArtist)
		album.genresMissing = genresMissing != 0
		album.trackCountMissing = trackCountMissing != 0
		if album.cleanTitle == "" {
			cleanTitle, err := utils.NormalizeSearchText(album.title)
			if err != nil {
				return nil, fmt.Errorf("failed to normalize album title %q: %w", album.title, err)
			}
			album.cleanTitle = cleanTitle
		}
		if album.cleanArtist == "" {
			cleanArtist, err := utils.NormalizeSearchText(album.artist)
			if err != nil {
				return nil, fmt.Errorf("failed to normalize album artist %q: %w", album.artist, err)
			}
			album.cleanArtist = cleanArtist
		}
		if album.id == "" || album.cleanTitle == "" || album.cleanArtist == "" {
			continue
		}

		albums = append(albums, album)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return albums, nil
}

func (w *Worker) fetchAlbumMetadata(ctx context.Context, album albumRecord) (albumMetadata, error) {
	metadata, err := w.fetchMusicBrainzAlbumMetadata(ctx, album)
	if err != nil {
		return albumMetadata{}, err
	}
	if metadata.genres == nil {
		metadata.genres = []string{}
	}
	if w.config.MaxGenres > 0 && len(metadata.genres) > w.config.MaxGenres {
		metadata.genres = metadata.genres[:w.config.MaxGenres]
	}

	return metadata, nil
}

func (w *Worker) FetchArtistGenres(ctx context.Context, artist string) ([]string, error) {
	musicBrainzGenres, err := w.fetchMusicBrainzGenres(ctx, artist)
	if err != nil {
		return nil, err
	}

	lastFMGenres, err := w.fetchLastFMGenres(ctx, artist)
	if err != nil {
		if errors.Is(err, errArtistNotFound) && len(musicBrainzGenres) > 0 {
			lastFMGenres = nil
		} else {
			return nil, err
		}
	}

	genres := mergeGenres(musicBrainzGenres, lastFMGenres)
	if len(genres) == 0 {
		return nil, errArtistNotFound
	}
	if w.config.MaxGenres > 0 && len(genres) > w.config.MaxGenres {
		genres = genres[:w.config.MaxGenres]
	}

	return genres, nil
}

func withDefaults(config Config) Config {
	defaults := DefaultConfig()
	if config.MusicBrainzBaseURL == "" {
		config.MusicBrainzBaseURL = defaults.MusicBrainzBaseURL
	}
	if config.LastFMBaseURL == "" {
		config.LastFMBaseURL = defaults.LastFMBaseURL
	}
	if config.UserAgent == "" {
		config.UserAgent = defaults.UserAgent
	}
	if config.MaxGenres == 0 {
		config.MaxGenres = defaults.MaxGenres
	}
	if config.DisableRateLimit {
		config.RequestDelay = 0
	} else if config.RequestDelay <= 0 {
		config.RequestDelay = defaults.RequestDelay
	}
	return config
}

func (w *Worker) artistsNeedingGenres(ctx context.Context) ([]artistRecord, error) {
	const stmt = `
		SELECT artist, clean_artist
		FROM tracks
		WHERE genres IS NULL AND TRIM(artist) != ''
		ORDER BY artist COLLATE NOCASE`

	rows, err := w.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("failed to query artists needing genres: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var artists []artistRecord
	for rows.Next() {
		var artist artistRecord
		if err := rows.Scan(&artist.name, &artist.cleanArtist); err != nil {
			return nil, err
		}

		artist.name = strings.TrimSpace(artist.name)
		artist.cleanArtist = strings.TrimSpace(artist.cleanArtist)
		if artist.cleanArtist == "" {
			cleanArtist, err := utils.NormalizeSearchText(artist.name)
			if err != nil {
				return nil, fmt.Errorf("failed to normalize artist %q: %w", artist.name, err)
			}
			artist.cleanArtist = cleanArtist
		}
		if artist.name == "" || artist.cleanArtist == "" || seen[artist.cleanArtist] {
			continue
		}

		seen[artist.cleanArtist] = true
		artists = append(artists, artist)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return artists, nil
}

func (w *Worker) fetchMusicBrainzAlbumMetadata(ctx context.Context, album albumRecord) (albumMetadata, error) {
	baseURL := strings.TrimRight(w.config.MusicBrainzBaseURL, "/")

	query := url.Values{}
	query.Set("query", fmt.Sprintf("release:%q AND artist:%q", album.cleanTitle, album.cleanArtist))
	query.Set("fmt", "json")
	query.Set("limit", "1")

	var search musicBrainzReleaseSearchResponse
	if err := w.getJSON(ctx, baseURL+"/release?"+query.Encode(), &search); err != nil {
		return albumMetadata{}, fmt.Errorf("musicbrainz release search failed: %w", err)
	}
	if len(search.Releases) == 0 || strings.TrimSpace(search.Releases[0].ID) == "" {
		return albumMetadata{}, errArtistNotFound
	}

	query = url.Values{}
	query.Set("fmt", "json")
	query.Set("inc", "genres+tags+media")

	var release musicBrainzReleaseResponse
	releaseURL := baseURL + "/release/" + url.PathEscape(search.Releases[0].ID) + "?" + query.Encode()
	if err := w.getJSON(ctx, releaseURL, &release); err != nil {
		return albumMetadata{}, fmt.Errorf("musicbrainz release lookup failed: %w", err)
	}

	return albumMetadata{
		genres:     mergeGenres(orderedTagNames(release.Genres, nil), orderedTagNames(release.Tags, nil)),
		trackCount: releaseTrackCount(release.Media),
	}, nil
}

func (w *Worker) fetchMusicBrainzGenres(ctx context.Context, artist string) ([]string, error) {
	baseURL := strings.TrimRight(w.config.MusicBrainzBaseURL, "/")

	query := url.Values{}
	query.Set("query", fmt.Sprintf("artist:%q", artist))
	query.Set("fmt", "json")
	query.Set("limit", "1")

	var search musicBrainzSearchResponse
	if err := w.getJSON(ctx, baseURL+"/artist?"+query.Encode(), &search); err != nil {
		return nil, fmt.Errorf("musicbrainz artist search failed: %w", err)
	}
	if len(search.Artists) == 0 || strings.TrimSpace(search.Artists[0].ID) == "" {
		return nil, errArtistNotFound
	}

	query = url.Values{}
	query.Set("fmt", "json")
	query.Set("inc", "genres+tags")

	var artistDetails musicBrainzArtistResponse
	artistURL := baseURL + "/artist/" + url.PathEscape(search.Artists[0].ID) + "?" + query.Encode()
	if err := w.getJSON(ctx, artistURL, &artistDetails); err != nil {
		return nil, fmt.Errorf("musicbrainz artist lookup failed: %w", err)
	}

	return mergeGenres(orderedTagNames(artistDetails.Genres, nil), orderedTagNames(artistDetails.Tags, nil)), nil
}

func (w *Worker) fetchLastFMGenres(ctx context.Context, artist string) ([]string, error) {
	if strings.TrimSpace(w.config.LastFMAPIKey) == "" {
		return nil, nil
	}

	query := url.Values{}
	query.Set("method", "artist.gettoptags")
	query.Set("artist", artist)
	query.Set("api_key", w.config.LastFMAPIKey)
	query.Set("format", "json")

	endpoint := appendQuery(w.config.LastFMBaseURL, query)

	var response lastFMTopTagsResponse
	if err := w.getJSON(ctx, endpoint, &response); err != nil {
		return nil, fmt.Errorf("last.fm top tags lookup failed: %w", err)
	}
	if response.Error == lastFMArtistNotFoundCode {
		return nil, errArtistNotFound
	}
	if response.Error != 0 {
		return nil, fmt.Errorf("last.fm returned error %d: %s", response.Error, response.Message)
	}

	return orderedTagNames(response.TopTags.Tags, isAllowedLastFMTag), nil
}

func (w *Worker) getJSON(ctx context.Context, endpoint string, target any) error {
	w.requestMu.Lock()
	defer w.requestMu.Unlock()

	if err := w.waitForRateLimit(ctx); err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", w.config.UserAgent)
	requestLabel := request.URL.Scheme + "://" + request.URL.Host + request.URL.Path

	response, err := w.http.Do(request)
	defer func() {
		w.lastCall = time.Now()
	}()
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		if response.StatusCode == http.StatusNotFound {
			return errArtistNotFound
		}
		return fmt.Errorf("GET %s returned HTTP %d: %s", requestLabel, response.StatusCode, strings.TrimSpace(string(body)))
	}

	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("failed to decode JSON from %s: %w", requestLabel, err)
	}

	return nil
}

func (w *Worker) waitForRateLimit(ctx context.Context) error {
	if w.config.RequestDelay <= 0 || w.lastCall.IsZero() {
		return nil
	}

	remaining := w.config.RequestDelay - time.Since(w.lastCall)
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

func (w *Worker) commitAlbumMetadata(ctx context.Context, album albumRecord, metadata albumMetadata) (int64, error) {
	updateGenres := 0
	if album.genresMissing {
		updateGenres = 1
	}

	updateTrackCount := 0
	var trackCount any
	if album.trackCountMissing && metadata.trackCount.Valid {
		updateTrackCount = 1
		trackCount = metadata.trackCount.Int64
	}

	if updateGenres == 0 && updateTrackCount == 0 {
		return 0, nil
	}

	genres := metadata.genres
	if genres == nil {
		genres = []string{}
	}
	payload, err := json.Marshal(genres)
	if err != nil {
		return 0, err
	}

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(
		ctx,
		`UPDATE albums
		SET
			genres = CASE WHEN ? = 1 AND genres IS NULL THEN ? ELSE genres END,
			track_count = CASE WHEN ? = 1 AND track_count IS NULL THEN ? ELSE track_count END
		WHERE id = ?
			AND ((? = 1 AND genres IS NULL) OR (? = 1 AND track_count IS NULL))`,
		updateGenres,
		string(payload),
		updateTrackCount,
		trackCount,
		album.id,
		updateGenres,
		updateTrackCount,
	)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return rowsAffected, nil
}

func (w *Worker) inheritTrackGenresFromAlbums(ctx context.Context) (int64, error) {
	result, err := w.db.ExecContext(ctx, `
		UPDATE tracks
		SET genres = (
			SELECT albums.genres
			FROM albums
			WHERE albums.id = tracks.album_id
		)
		WHERE tracks.genres IS NULL
			AND tracks.album_id IS NOT NULL
			AND EXISTS (
				SELECT 1
				FROM albums
				WHERE albums.id = tracks.album_id
					AND albums.genres IS NOT NULL
			)`)
	if err != nil {
		return 0, fmt.Errorf("failed to inherit track genres from albums: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rowsAffected, nil
}

func (w *Worker) commitTrackGenres(ctx context.Context, artist artistRecord, genres []string) (int64, error) {
	payload, err := json.Marshal(genres)
	if err != nil {
		return 0, err
	}

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(
		ctx,
		"UPDATE tracks SET genres = ? WHERE genres IS NULL AND clean_artist = ?",
		string(payload),
		artist.cleanArtist,
	)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return rowsAffected, nil
}

func releaseTrackCount(media []musicBrainzMedium) sql.NullInt64 {
	var total int64
	for _, medium := range media {
		switch {
		case medium.TrackCount > 0:
			total += int64(medium.TrackCount)
		case len(medium.Tracks) > 0:
			total += int64(len(medium.Tracks))
		}
	}
	if total == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: total, Valid: true}
}

func formatNullableTrackCount(value sql.NullInt64) string {
	if !value.Valid {
		return "NULL"
	}
	return strconv.FormatInt(value.Int64, 10)
}

func orderedTagNames(tags []apiTag, filter func(string) bool) []string {
	candidates := make([]genreCandidate, 0, len(tags))
	for idx, tag := range tags {
		name := normalizeGenreName(tag.Name)
		if name == "" || (filter != nil && !filter(name)) {
			continue
		}
		candidates = append(candidates, genreCandidate{
			name:  name,
			count: int(tag.Count),
			index: idx,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].count == candidates[j].count {
			return candidates[i].index < candidates[j].index
		}
		return candidates[i].count > candidates[j].count
	})

	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		names = append(names, candidate.name)
	}

	return mergeGenres(names)
}

func isAllowedLastFMTag(name string) bool {
	name = normalizeGenreName(name)
	for _, allowed := range lastFMGenreWhitelist {
		if strings.Contains(name, allowed) {
			return true
		}
	}
	return false
}

func mergeGenres(groups ...[]string) []string {
	seen := make(map[string]bool)
	var genres []string
	for _, group := range groups {
		for _, value := range group {
			name := normalizeGenreName(value)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			genres = append(genres, name)
		}
	}

	return genres
}

func appendQuery(base string, query url.Values) string {
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	return base + separator + query.Encode()
}

func normalizeGenreName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}

func (w *Worker) logf(format string, args ...any) {
	if w.config.Logger == nil {
		return
	}
	w.config.Logger.Printf(format, args...)
}

func (w *Worker) warnf(format string, args ...any) {
	if w.config.Logger != nil {
		w.config.Logger.Printf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func (c *flexibleCount) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		*c = 0
		return nil
	}

	var text string
	if strings.HasPrefix(raw, `"`) {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
	} else {
		text = raw
	}

	text = strings.TrimSpace(text)
	if text == "" {
		*c = 0
		return nil
	}

	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		*c = 0
		return nil
	}

	*c = flexibleCount(int(value))
	return nil
}
