package database

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

const maxRecommendationContextRecords = 24

// RecommendationContextRecord is a bounded, request-relevant local signal.
// SignalType is deliberately explicit: listening_activity is never a fit
// judgment and taste_feedback is distinct from prompt_fit.
type RecommendationContextRecord struct {
	Source     string `json:"source"`
	Entity     string `json:"entity"`
	Artist     string `json:"artist,omitempty"`
	Album      string `json:"album,omitempty"`
	Song       string `json:"song,omitempty"`
	SignalType string `json:"signal_type"`
	Value      string `json:"value"`
	Details    string `json:"details,omitempty"`
	Relevance  int    `json:"-"`
}

// FetchRelevantRecommendationContext returns a small set of local records
// matching supplied reference/neighbor names. Empty terms return sparse
// context rather than global history; callers may separately provide broad
// profile summaries for calibration.
func FetchRelevantRecommendationContext(ctx context.Context, db *sql.DB, terms []string, limit int) ([]RecommendationContextRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	terms = compactContextTerms(terms)
	if len(terms) == 0 {
		return []RecommendationContextRecord{}, nil
	}
	if limit <= 0 || limit > maxRecommendationContextRecords {
		limit = maxRecommendationContextRecords
	}
	rows, err := db.QueryContext(ctx, `
		SELECT source, entity, artist, album, song, signal_type, value, details FROM (
			SELECT 'tracks' AS source, 'track' AS entity, artist AS artist, COALESCE(album, '') AS album, title AS song,
				'favorite' AS signal_type, 'favorite' AS value, '' AS details
			FROM tracks WHERE is_favorite = 1 AND (clean_artist IN (`+placeholders(len(terms))+`) OR clean_title IN (`+placeholders(len(terms))+`))
			UNION ALL
			SELECT 'albums', 'album', artist, title, '', 'album_rating', CAST(user_rating AS TEXT), ''
			FROM albums WHERE user_rating IS NOT NULL AND (clean_artist IN (`+placeholders(len(terms))+`) OR clean_title IN (`+placeholders(len(terms))+`))
			UNION ALL
			SELECT 'recommendation_feedback', 'album', artist, album, COALESCE(starter_track, ''), 'taste_feedback', verdict, COALESCE(notes, '')
			FROM recommendation_feedback WHERE clean_artist IN (`+placeholders(len(terms))+`) OR clean_title IN (`+placeholders(len(terms))+`)
			UNION ALL
			SELECT 'recommendation_prompt_fit_feedback', 'candidate', c.artist, c.album, COALESCE(c.song, ''), 'prompt_fit', f.verdict, COALESCE(f.notes, '')
			FROM recommendation_prompt_fit_feedback f JOIN recommendation_candidates c ON c.id = f.candidate_id
			WHERE c.clean_artist IN (`+placeholders(len(terms))+`) OR c.clean_title IN (`+placeholders(len(terms))+`)
			UNION ALL
			SELECT 'apple_music_play_activity', 'listening_activity', COALESCE(artist, ''), COALESCE(album, ''), COALESCE(song_name, ''), 'listening_activity', CAST(COUNT(*) AS TEXT), 'play events; not a fit judgment'
			FROM apple_music_play_activity
			WHERE lower(trim(COALESCE(artist, ''))) IN (`+placeholders(len(terms))+`) OR lower(trim(COALESCE(album, ''))) IN (`+placeholders(len(terms))+`)
			GROUP BY lower(trim(COALESCE(artist, ''))), lower(trim(COALESCE(album, ''))), song_name
		) ORDER BY signal_type, artist COLLATE NOCASE, album COLLATE NOCASE LIMIT ?`, contextArgs(terms, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RecommendationContextRecord
	for rows.Next() {
		var item RecommendationContextRecord
		if err := rows.Scan(&item.Source, &item.Entity, &item.Artist, &item.Album, &item.Song, &item.SignalType, &item.Value, &item.Details); err != nil {
			return nil, err
		}
		item.Relevance = contextRecordRelevance(item, terms)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Relevance > result[j].Relevance })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// FindExactContextNames returns normalized local artist/album names that occur
// as complete phrases in request text. It deliberately does not tokenize prose
// into individual words, avoiding broad or unrelated history matches.
func FindExactContextNames(ctx context.Context, db *sql.DB, requestText string, limit int) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	requestText = strings.ToLower(strings.TrimSpace(requestText))
	if requestText == "" {
		return []string{}, nil
	}
	if limit <= 0 || limit > maxRecommendationContextRecords {
		limit = maxRecommendationContextRecords
	}
	rows, err := db.QueryContext(ctx, `
		SELECT clean_artist FROM albums WHERE clean_artist != ''
		UNION SELECT clean_title FROM albums WHERE clean_title != ''
		UNION SELECT clean_artist FROM tracks WHERE clean_artist != ''
		UNION SELECT clean_title FROM tracks WHERE clean_title != ''
		ORDER BY 1 LIMIT ?`, limit*4)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := make(map[string]bool)
	result := make([]string, 0, limit)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || seen[name] || !containsExactPhrase(requestText, name) {
			continue
		}
		seen[name] = true
		result = append(result, name)
		if len(result) == limit {
			break
		}
	}
	return result, rows.Err()
}

func containsExactPhrase(text, phrase string) bool {
	text = " " + strings.Join(strings.Fields(text), " ") + " "
	phrase = " " + strings.Join(strings.Fields(phrase), " ") + " "
	return strings.Contains(text, phrase)
}

func compactContextTerms(terms []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" || seen[term] {
			continue
		}
		seen[term] = true
		result = append(result, term)
	}
	return result
}

func placeholders(count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func contextArgs(terms []string, limit int) []any {
	args := make([]any, 0, len(terms)*10+1)
	// The first four UNION branches each have one IN pair; the activity branch
	// has one artist/album pair. Keep the construction explicit and positional.
	args = args[:0]
	for branch := 0; branch < 4; branch++ {
		for _, term := range terms {
			args = append(args, term)
		}
		for _, term := range terms {
			args = append(args, term)
		}
	}
	for _, term := range terms {
		args = append(args, term)
	}
	for _, term := range terms {
		args = append(args, term)
	}
	args = append(args, limit)
	return args
}

func contextRecordRelevance(item RecommendationContextRecord, terms []string) int {
	text := strings.ToLower(strings.Join([]string{item.Artist, item.Album, item.Song}, " "))
	score := 1
	for _, term := range terms {
		if strings.EqualFold(strings.TrimSpace(item.Artist), term) {
			score += 10
		}
		if strings.EqualFold(strings.TrimSpace(item.Album), term) {
			score += 8
		}
		if strings.Contains(strings.ToLower(item.Artist), term) || strings.Contains(strings.ToLower(item.Album), term) || strings.Contains(text, term) {
			score += 2
		}
	}
	if item.SignalType == "prompt_fit" || item.SignalType == "taste_feedback" {
		score++
	}
	return score
}
