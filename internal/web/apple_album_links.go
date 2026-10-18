package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nicksunday/music-context-platform/internal/database"
)

type cachedAppleAlbum struct {
	url     string
	expires time.Time
}

// Remove only recognizable edition qualifiers. Live albums, tributes, sequels,
// and unrelated albums with a shared title suffix must remain distinct.
var appleAlbumEdition = regexp.MustCompile(`(?i)\s*[\[(](?:(?:super\s+)?deluxe(?:\s+edition|\s+version)?|expanded(?:\s+edition|\s+version)?|special\s+edition|(?:\d+(?:st|nd|rd|th)\s+)?anniversary(?:\s+edition)?|(?:\d{4}\s+)?remaster(?:ed)?(?:\s+\d{4})?)[\])]\s*$`)

var appleAlbumFormat = regexp.MustCompile(`(?i)\s+-\s+(single|ep)\s*$`)
var appleAlbumAnchor = regexp.MustCompile(`(?is)<a\b[^>]*\bhref=["']([^"']+)["'][^>]*>(.*?)</a>`)
var appleAlbumHTMLTags = regexp.MustCompile(`<[^>]+>`)

func appleAlbumTitle(value string) string {
	value = appleAlbumFormat.ReplaceAllString(value, "")
	for {
		stripped := appleAlbumEdition.ReplaceAllString(value, "")
		if stripped == value {
			break
		}
		value = stripped
	}
	return canonicalAlbumTitle(value)
}

func (linker *AppleMusicLinker) resolveAlbum(ctx context.Context, candidate database.RecommendationCandidateInput) (string, error) {
	if linker == nil || linker.client == nil {
		return "", nil
	}
	artist, album, err := database.NormalizeAlbumLookup(candidate.Artist, candidate.Album)
	if err != nil || artist == "" || album == "" {
		return "", err
	}
	key := artist + "\x00" + album
	linker.albumCacheMu.Lock()
	cached := linker.albumCache[key]
	linker.albumCacheMu.Unlock()
	if cached.url != "" && time.Now().Before(cached.expires) {
		return cached.url, nil
	}

	// Apple may return no results for a long combined query even when the album
	// appears in a title search or in the artist's catalog.
	terms := compactWebStrings([]string{candidate.Artist + " " + candidate.Album, appleAlbumTitle(candidate.Album), candidate.Artist})
	var firstErr error
	var artistID int64
	for _, term := range terms {
		results, err := linker.searchAlbums(ctx, term)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if ctx.Err() != nil {
				break
			}
			continue
		}
		bestURL, bestScore := "", 0
		for _, result := range results {
			resultArtist, _, err := database.NormalizeAlbumLookup(result.ArtistName, result.CollectionName)
			if err != nil || resultArtist != artist {
				continue
			}
			if artistID == 0 && result.ArtistID > 0 {
				artistID = result.ArtistID
			}
			score := appleAlbumMatchScore(candidate.Album, result.CollectionName)
			parsed, err := url.Parse(strings.TrimSpace(result.CollectionViewURL))
			if score <= bestScore || err != nil || parsed.Scheme != "https" || parsed.Host != "music.apple.com" || parsed.User != nil || !strings.Contains(parsed.Path, "/album/") {
				continue
			}
			bestURL, bestScore = parsed.String(), score
		}
		if bestURL != "" {
			linker.cacheAlbum(key, bestURL)
			return bestURL, nil
		}
	}
	if artistID > 0 {
		resolved, err := linker.albumFromArtistPage(ctx, artistID, candidate.Album)
		if resolved != "" {
			linker.cacheAlbum(key, resolved)
			return resolved, nil
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return "", firstErr
}

func (linker *AppleMusicLinker) searchAlbums(ctx context.Context, term string) ([]iTunesSearchResult, error) {
	endpoint, err := url.Parse(linker.baseURL)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("term", term)
	query.Set("media", "music")
	query.Set("entity", "album")
	query.Set("limit", "200")
	endpoint.RawQuery = query.Encode()
	if err := linker.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", webUserAgent)
	response, err := linker.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("iTunes returned %s", response.Status)
	}
	var payload iTunesSearchResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Results, nil
}

func appleAlbumMatchScore(target, result string) int {
	if canonicalAlbumTitle(target) == canonicalAlbumTitle(result) {
		return 2
	}
	if title := appleAlbumTitle(target); title != "" && title == appleAlbumTitle(result) {
		return 1
	}
	return 0
}

func (linker *AppleMusicLinker) cacheAlbum(key, destination string) {
	linker.albumCacheMu.Lock()
	defer linker.albumCacheMu.Unlock()
	if linker.albumCache == nil || len(linker.albumCache) >= 512 {
		linker.albumCache = make(map[string]cachedAppleAlbum)
	}
	linker.albumCache[key] = cachedAppleAlbum{url: destination, expires: time.Now().Add(24 * time.Hour)}
}

// Apple's public artist catalog can contain new albums absent from the older
// iTunes search index. Only visit an artist ID confirmed by an exact artist match.
func (linker *AppleMusicLinker) albumFromArtistPage(ctx context.Context, artistID int64, album string) (string, error) {
	base := linker.catalogBaseURL
	if base == "" {
		base = "https://music.apple.com"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/us/artist/"+strconv.FormatInt(artistID, 10), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", webUserAgent)
	response, err := linker.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Apple Music catalog returned %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	bestURL, bestScore := "", 0
	for _, anchor := range appleAlbumAnchor.FindAllSubmatch(body, -1) {
		title := html.UnescapeString(appleAlbumHTMLTags.ReplaceAllString(string(anchor[2]), " "))
		score := appleAlbumMatchScore(album, title)
		parsed, err := url.Parse(html.UnescapeString(string(anchor[1])))
		if score <= bestScore || err != nil || parsed.Scheme != "https" || parsed.Host != "music.apple.com" || parsed.User != nil || !strings.Contains(parsed.Path, "/album/") {
			continue
		}
		bestURL, bestScore = parsed.String(), score
	}
	return bestURL, nil
}
