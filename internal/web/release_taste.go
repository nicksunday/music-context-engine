package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/nicksunday/music-context-platform/internal/database"
)

// WithTasteProfile enables discovery beyond artists already liked. The profile
// is read during background refreshes, keeping page requests inexpensive.
func (radar *MusicBrainzReleaseRadar) WithTasteProfile(db *sql.DB) *MusicBrainzReleaseRadar {
	radar.tasteDB = db
	return radar
}

func (radar *MusicBrainzReleaseRadar) releaseTasteGenres(ctx context.Context) ([]string, error) {
	if radar.tasteDB == nil {
		return nil, nil
	}
	genres, err := database.FetchAllGenreTopography(ctx, radar.tasteDB)
	if err != nil {
		return nil, err
	}
	var preferred []string
	for _, genre := range genres {
		if genre.FavoriteTracksCount > genre.DislikedTracksCount || (genre.AvgAlbumRating.Valid && genre.AvgAlbumRating.Float64 >= 4) {
			if tag := specificReleaseGenre(genre.Subgenre); tag != "" {
				preferred = append(preferred, tag)
			}
		}
	}
	sort.Strings(preferred)
	return compactWebStrings(preferred), nil
}

func (radar *MusicBrainzReleaseRadar) releaseBlockedArtists(ctx context.Context) (map[string]bool, error) {
	blocked := make(map[string]bool)
	if radar.tasteDB == nil {
		return blocked, nil
	}
	rows, err := radar.tasteDB.QueryContext(ctx, `SELECT clean_artist FROM v_artist_affinity
  WHERE (curved_affinity_score < 0 OR avg_user_rating < 3) AND favorite_tracks_count = 0
  AND (avg_user_rating IS NULL OR avg_user_rating < 4)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		blocked[name] = true
	}
	return blocked, rows.Err()
}

func specificReleaseGenre(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	// These describe too much music to establish a useful discovery connection.
	switch value {
	case "", "metal", "rock", "pop", "electronic", "experimental", "alternative", "indie", "american", "british", "english", "german", "finnish", "swedish", "norwegian", "canadian", "australian", "french", "japanese", "russian", "italian", "dutch", "scottish", "spanish", "danish", "irish", "polish", "brazilian", "instrumental", "seen live", "male vocalists", "female vocalists", "melodic", "epic", "symphonic", "progressive", "technical", "atmospheric", "dark":
		return ""
	}
	if !isUsefulMusicBrainzTag(value) {
		return ""
	}
	return value
}

// Families express loose style adjacency, not a claim that two artists sound
// identical. A single liked subgenre can open the neighboring styles.
func releaseGenreFamily(tag string) string {
	switch tag {
	case "power metal", "symphonic metal", "symphonic power metal", "folk metal", "viking metal", "pagan metal", "progressive metal", "neoclassical metal", "epic metal", "melodic metal":
		return "melodic metal"
	case "progressive rock", "psychedelic rock", "space rock", "art rock", "neo-progressive rock":
		return "progressive rock"
	case "technical death metal", "progressive death metal", "melodic death metal", "death metal":
		return "death metal"
	}
	return ""
}

func releaseTasteReason(tags, preferred []string) string {
	for _, tag := range tags {
		tag = specificReleaseGenre(tag)
		if tag == "" {
			continue
		}
		for _, liked := range preferred {
			if tag == liked {
				return "Matches " + tag + " in your liked music."
			}
		}
	}
	for _, tag := range tags {
		tag = specificReleaseGenre(tag)
		family := releaseGenreFamily(tag)
		if family == "" {
			continue
		}
		for _, liked := range preferred {
			if releaseGenreFamily(liked) == family {
				return "Explore " + tag + " — adjacent to " + liked + " in your liked music."
			}
		}
	}
	return ""
}

// Multiple matching styles strengthen a discovery within the same release day.
func releaseTasteStrength(tags, preferred []string) int {
	strength := 0
	for _, tag := range compactWebStrings(tags) {
		reason := releaseTasteReason([]string{tag}, preferred)
		if strings.HasPrefix(reason, "Matches ") {
			strength += 2
		} else if reason != "" {
			strength++
		}
	}
	return min(strength, 20)
}

// Newly released albums often have no release-group tags yet. Resolve artist
// tags in batches, newest albums first, rather than requiring an album tag or
// making one request for each release in the full 90-day feed.
func (radar *MusicBrainzReleaseRadar) freshArtistTags(ctx context.Context, releases []listenBrainzFreshRelease, since, until time.Time, preferred []string, known map[string]bool) (map[string][]string, error) {
	tags := make(map[string][]string)
	if len(preferred) == 0 {
		return tags, nil
	}
	releases = append([]listenBrainzFreshRelease(nil), releases...)
	sort.SliceStable(releases, func(i, j int) bool {
		if releases[i].ReleaseDate != releases[j].ReleaseDate {
			return releases[i].ReleaseDate > releases[j].ReleaseDate
		}
		return strings.EqualFold(releases[i].ReleaseGroupPrimaryType, "album") && !strings.EqualFold(releases[j].ReleaseGroupPrimaryType, "album")
	})
	var ids []string
	seen := make(map[string]bool)
	for _, release := range releases {
		date, ok := parseMusicBrainzDate(release.ReleaseDate)
		clean, _ := normalizedArtistName(release.ArtistCreditName)
		if !ok || date.Before(dayStart(since)) || date.After(dayStart(until)) || !isRadarReleaseType(release.ReleaseGroupPrimaryType) || artistMatchesAffinitySet(release.ArtistCreditName, clean, known) || releaseTasteReason(release.ReleaseTags, preferred) != "" {
			continue
		}
		for _, id := range release.ArtistMBIDs {
			if !validReleaseArtistID(id) || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
			if len(ids) >= 400 {
				break
			}
		}
		if len(ids) >= 400 {
			break
		}
	}
	var firstErr error
	for start := 0; start < len(ids); start += 100 {
		batch := ids[start:min(start+100, len(ids))]
		results, err := radar.searchReleaseArtistTags(ctx, batch)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for id, values := range results {
			tags[id] = values
		}
	}
	return tags, firstErr
}

func validReleaseArtistID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func (radar *MusicBrainzReleaseRadar) searchReleaseArtistTags(ctx context.Context, ids []string) (map[string][]string, error) {
	endpoint, err := url.JoinPath(radar.baseURL, "artist")
	if err != nil {
		return nil, err
	}
	terms := make([]string, 0, len(ids))
	requested := make(map[string]bool)
	for _, id := range ids {
		terms = append(terms, "arid:"+id)
		requested[id] = true
	}
	query := url.Values{"query": {strings.Join(terms, " OR ")}, "fmt": {"json"}, "limit": {"100"}}
	if err := radar.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", webUserAgent)
	response, err := radar.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artist tags returned %s", response.Status)
	}
	var payload struct {
		Artists []struct {
			ID   string           `json:"id"`
			Tags []musicBrainzTag `json:"tags"`
		} `json:"artists"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	results := make(map[string][]string)
	for _, artist := range payload.Artists {
		if requested[artist.ID] {
			results[artist.ID] = topMusicBrainzTags(artist.Tags, 12)
		}
	}
	return results, nil
}
