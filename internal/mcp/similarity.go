package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	lastFMAPIKeyEnv           = "LASTFM_API_KEY"
	defaultLastFMBaseURL      = "https://ws.audioscrobbler.com/2.0/"
	defaultLastFMTimeout      = 10 * time.Second
	defaultLastFMRequestDelay = 250 * time.Millisecond

	maxAdjacencySeedArtists  = 6
	maxSimilarArtistsPerSeed = 15
	maxRealAdjacentArtists   = 20
)

// similarArtist is one entry from Last.fm's artist.getsimilar response.
type similarArtist struct {
	Name  string
	Match float64
}

// similarArtistSource looks up artists similar to a given seed artist using
// real listening/tagging data, rather than asking the LLM to guess at
// adjacency from its own training knowledge. Implementations should return
// (nil, nil) for expected "no data" cases (missing API key, unknown artist)
// rather than an error, so one bad seed doesn't fail an entire lookup.
type similarArtistSource interface {
	SimilarArtists(ctx context.Context, artist string, limit int) ([]similarArtist, error)
}

type lastFMSimilarClientConfig struct {
	HTTPClient   *http.Client
	BaseURL      string
	APIKey       string
	Timeout      time.Duration
	RequestDelay time.Duration
}

type lastFMSimilarClient struct {
	httpClient   *http.Client
	baseURL      string
	apiKey       string
	timeout      time.Duration
	requestDelay time.Duration

	requestMu sync.Mutex
	lastCall  time.Time
}

func newLastFMSimilarClient(config lastFMSimilarClientConfig) *lastFMSimilarClient {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = defaultLastFMBaseURL
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultLastFMTimeout
	}
	if config.RequestDelay < 0 {
		config.RequestDelay = 0
	}

	return &lastFMSimilarClient{
		httpClient:   httpClient,
		baseURL:      config.BaseURL,
		apiKey:       strings.TrimSpace(config.APIKey),
		timeout:      config.Timeout,
		requestDelay: config.RequestDelay,
	}
}

// defaultSimilarArtistSource returns nil (not an error) when LASTFM_API_KEY
// isn't configured, so callers can skip real-adjacency lookups gracefully
// and fall back to affinity-only context instead of failing the tool call.
func defaultSimilarArtistSource() similarArtistSource {
	apiKey := strings.TrimSpace(os.Getenv(lastFMAPIKeyEnv))
	if apiKey == "" {
		return nil
	}
	return newLastFMSimilarClient(lastFMSimilarClientConfig{
		APIKey:       apiKey,
		RequestDelay: defaultLastFMRequestDelay,
	})
}

type lastFMSimilarArtistsResponse struct {
	SimilarArtists struct {
		Artist []struct {
			Name  string `json:"name"`
			Match string `json:"match"`
		} `json:"artist"`
	} `json:"similarartists"`
	Error   int    `json:"error"`
	Message string `json:"message"`
}

func (c *lastFMSimilarClient) SimilarArtists(ctx context.Context, artist string, limit int) ([]similarArtist, error) {
	artist = strings.TrimSpace(artist)
	if artist == "" || c.apiKey == "" {
		return nil, nil
	}
	if limit <= 0 || limit > maxSimilarArtistsPerSeed {
		limit = maxSimilarArtistsPerSeed
	}

	query := url.Values{}
	query.Set("method", "artist.getsimilar")
	query.Set("artist", artist)
	query.Set("api_key", c.apiKey)
	query.Set("format", "json")
	query.Set("limit", strconv.Itoa(limit))
	endpoint := strings.TrimRight(c.baseURL, "?") + "?" + query.Encode()

	var payload lastFMSimilarArtistsResponse
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	if payload.Error != 0 {
		// Unknown-artist and similar "not found" style errors are common for
		// obscure or misspelled names; treat as "no data" rather than a hard
		// failure so one bad seed artist doesn't sink the whole lookup.
		return nil, nil
	}

	results := make([]similarArtist, 0, len(payload.SimilarArtists.Artist))
	for _, entry := range payload.SimilarArtists.Artist {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		match, _ := strconv.ParseFloat(entry.Match, 64)
		results = append(results, similarArtist{Name: name, Match: match})
	}
	return results, nil
}

func (c *lastFMSimilarClient) getJSON(ctx context.Context, endpoint string, target any) error {
	c.requestMu.Lock()
	defer c.requestMu.Unlock()

	if err := c.waitForRateLimitLocked(ctx); err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(callCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")

	response, err := c.httpClient.Do(request)
	defer func() {
		c.lastCall = time.Now()
	}()
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("last.fm artist.getsimilar returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode last.fm artist.getsimilar response: %w", err)
	}
	return nil
}

func (c *lastFMSimilarClient) waitForRateLimitLocked(ctx context.Context) error {
	if c.requestDelay <= 0 || c.lastCall.IsZero() {
		return nil
	}
	elapsed := time.Since(c.lastCall)
	if elapsed >= c.requestDelay {
		return nil
	}
	select {
	case <-time.After(c.requestDelay - elapsed):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
