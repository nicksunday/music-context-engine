package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/nicksunday/music-context-platform/internal/utils"
)

type RecommendationCandidateInput struct {
	Artist       string   `json:"artist"`
	Album        string   `json:"album"`
	StarterTrack string   `json:"starter_track,omitempty"`
	ReleaseYear  int      `json:"release_year,omitempty"`
	GenreTags    []string `json:"genre_tags,omitempty"`
	Rank         int      `json:"rank,omitempty"`
	Note         string   `json:"note,omitempty"`
	StreamingURL string   `json:"streaming_url,omitempty"`
}

type RecommendationCandidate struct {
	ID           string   `json:"id"`
	BatchID      string   `json:"batch_id"`
	Artist       string   `json:"artist"`
	Album        string   `json:"album"`
	CleanArtist  string   `json:"clean_artist"`
	CleanTitle   string   `json:"clean_title"`
	StarterTrack string   `json:"starter_track,omitempty"`
	ReleaseYear  int      `json:"release_year,omitempty"`
	GenreTags    []string `json:"genre_tags,omitempty"`
	Rank         int      `json:"rank,omitempty"`
	Note         string   `json:"note,omitempty"`
	StreamingURL string   `json:"streaming_url,omitempty"`
}

type RecommendationBatchInput struct {
	Prompt     string                         `json:"prompt"`
	Mood       string                         `json:"mood,omitempty"`
	Notes      string                         `json:"notes,omitempty"`
	Candidates []RecommendationCandidateInput `json:"candidates"`
}

type RecommendationBatch struct {
	ID         string                    `json:"id"`
	Prompt     string                    `json:"prompt"`
	Mood       string                    `json:"mood,omitempty"`
	Notes      string                    `json:"notes,omitempty"`
	Candidates []RecommendationCandidate `json:"candidates"`
}

type RecommendationBatchSummary struct {
	ID             string `json:"id"`
	Prompt         string `json:"prompt"`
	Mood           string `json:"mood,omitempty"`
	CreatedAt      string `json:"created_at"`
	CandidateCount int    `json:"candidate_count"`
	Reply          string `json:"reply,omitempty"`
}

type RecommendationFeedbackInput struct {
	Artist       string `json:"artist"`
	Album        string `json:"album"`
	StarterTrack string `json:"starter_track,omitempty"`
	BatchID      string `json:"batch_id,omitempty"`
	CandidateID  string `json:"candidate_id,omitempty"`
	Verdict      string `json:"verdict"`
	Mood         string `json:"mood,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

type RecommendationFeedbackLog struct {
	ID           string `json:"id"`
	Artist       string `json:"artist"`
	Album        string `json:"album"`
	CleanArtist  string `json:"clean_artist"`
	CleanTitle   string `json:"clean_title"`
	StarterTrack string `json:"starter_track,omitempty"`
	BatchID      string `json:"batch_id,omitempty"`
	CandidateID  string `json:"candidate_id,omitempty"`
	Verdict      string `json:"verdict"`
	Mood         string `json:"mood,omitempty"`
	Notes        string `json:"notes,omitempty"`
	RowsInserted int64  `json:"rows_inserted,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
}

func CreateRecommendationBatch(ctx context.Context, db *sql.DB, input RecommendationBatchInput) (RecommendationBatch, error) {
	if db == nil {
		return RecommendationBatch{}, fmt.Errorf("database is not initialized")
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return RecommendationBatch{}, fmt.Errorf("recommendation prompt must not be empty")
	}

	batch := RecommendationBatch{
		ID:     uuid.NewString(),
		Prompt: prompt,
		Mood:   strings.TrimSpace(input.Mood),
		Notes:  strings.TrimSpace(input.Notes),
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return RecommendationBatch{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO recommendation_batches (id, prompt, mood, notes)
		VALUES (?, ?, ?, ?)`,
		batch.ID,
		nullableTrimmedString(batch.Prompt),
		nullableTrimmedString(batch.Mood),
		nullableTrimmedString(batch.Notes),
	); err != nil {
		return RecommendationBatch{}, err
	}

	for idx, candidateInput := range input.Candidates {
		candidate, err := normalizeRecommendationCandidate(batch.ID, idx+1, candidateInput)
		if err != nil {
			return RecommendationBatch{}, err
		}
		if candidate.CleanArtist == "" || candidate.CleanTitle == "" {
			continue
		}

		rawGenres, err := json.Marshal(candidate.GenreTags)
		if err != nil {
			return RecommendationBatch{}, fmt.Errorf("marshal recommendation candidate genre tags: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO recommendation_candidates (
				id, batch_id, artist, album, clean_artist, clean_title, starter_track, release_year, genre_tags, rank, streaming_url
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			candidate.ID,
			candidate.BatchID,
			candidate.Artist,
			candidate.Album,
			candidate.CleanArtist,
			candidate.CleanTitle,
			nullableTrimmedString(candidate.StarterTrack),
			nullablePositiveInt(candidate.ReleaseYear),
			string(rawGenres),
			candidate.Rank,
			nullableTrimmedString(candidate.StreamingURL),
		); err != nil {
			return RecommendationBatch{}, err
		}

		batch.Candidates = append(batch.Candidates, candidate)
	}

	if err := tx.Commit(); err != nil {
		return RecommendationBatch{}, err
	}

	return batch, nil
}

func LogRecommendationFeedback(ctx context.Context, db *sql.DB, input RecommendationFeedbackInput) (RecommendationFeedbackLog, error) {
	if db == nil {
		return RecommendationFeedbackLog{}, fmt.Errorf("database is not initialized")
	}
	canonicalVerdict, ok := CanonicalRecommendationVerdict(input.Verdict)
	if !ok {
		return RecommendationFeedbackLog{}, fmt.Errorf("unsupported recommendation feedback verdict %q", input.Verdict)
	}

	cleanArtist, cleanTitle, err := NormalizeAlbumLookup(input.Artist, input.Album)
	if err != nil {
		return RecommendationFeedbackLog{}, err
	}

	result := RecommendationFeedbackLog{
		ID:           uuid.NewString(),
		Artist:       strings.TrimSpace(input.Artist),
		Album:        strings.TrimSpace(input.Album),
		CleanArtist:  cleanArtist,
		CleanTitle:   cleanTitle,
		StarterTrack: strings.TrimSpace(input.StarterTrack),
		BatchID:      strings.TrimSpace(input.BatchID),
		CandidateID:  strings.TrimSpace(input.CandidateID),
		Verdict:      canonicalVerdict,
		Mood:         strings.TrimSpace(input.Mood),
		Notes:        strings.TrimSpace(input.Notes),
	}
	if cleanArtist == "" || cleanTitle == "" {
		return result, nil
	}

	insertResult, err := db.ExecContext(ctx, `
		INSERT INTO recommendation_feedback (
			id, batch_id, candidate_id, artist, album, clean_artist, clean_title, starter_track, verdict, mood, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		result.ID,
		nullableTrimmedString(result.BatchID),
		nullableTrimmedString(result.CandidateID),
		result.Artist,
		result.Album,
		result.CleanArtist,
		result.CleanTitle,
		nullableTrimmedString(result.StarterTrack),
		result.Verdict,
		nullableTrimmedString(result.Mood),
		nullableTrimmedString(result.Notes),
	)
	if err != nil {
		return RecommendationFeedbackLog{}, err
	}
	result.RowsInserted, err = insertResult.RowsAffected()
	if err != nil {
		return RecommendationFeedbackLog{}, err
	}

	return result, nil
}

func FetchRecentRecommendationFeedback(ctx context.Context, db *sql.DB, limit int) ([]RecommendationFeedbackLog, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id, artist, album, clean_artist, clean_title, starter_track, batch_id, candidate_id, verdict, mood, notes, created_at
		FROM recommendation_feedback
		ORDER BY created_at DESC, rowid DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feedback []RecommendationFeedbackLog
	for rows.Next() {
		var row RecommendationFeedbackLog
		var starterTrack, batchID, candidateID, mood, notes, createdAt sql.NullString
		if err := rows.Scan(
			&row.ID,
			&row.Artist,
			&row.Album,
			&row.CleanArtist,
			&row.CleanTitle,
			&starterTrack,
			&batchID,
			&candidateID,
			&row.Verdict,
			&mood,
			&notes,
			&createdAt,
		); err != nil {
			return nil, err
		}
		row.StarterTrack = nullStringText(starterTrack)
		row.BatchID = nullStringText(batchID)
		row.CandidateID = nullStringText(candidateID)
		row.Mood = nullStringText(mood)
		row.Notes = nullStringText(notes)
		row.CreatedAt = nullStringText(createdAt)
		feedback = append(feedback, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return feedback, nil
}

func FetchLatestRecommendationBatch(ctx context.Context, db *sql.DB) (RecommendationBatch, error) {
	if db == nil {
		return RecommendationBatch{}, fmt.Errorf("database is not initialized")
	}

	var id string
	err := db.QueryRowContext(ctx, `
		SELECT id FROM recommendation_batches
		ORDER BY created_at DESC, rowid DESC
		LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return RecommendationBatch{}, nil
	}
	if err != nil {
		return RecommendationBatch{}, err
	}

	return FetchRecommendationBatchByID(ctx, db, id)
}

func FetchRecommendationBatchByID(ctx context.Context, db *sql.DB, id string) (RecommendationBatch, error) {
	if db == nil {
		return RecommendationBatch{}, fmt.Errorf("database is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return RecommendationBatch{}, fmt.Errorf("recommendation batch id must not be empty")
	}

	var batch RecommendationBatch
	var prompt, mood, notes, createdAt sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT id, prompt, mood, notes, created_at FROM recommendation_batches WHERE id = ?`, id,
	).Scan(&batch.ID, &prompt, &mood, &notes, &createdAt)
	if err != nil {
		return RecommendationBatch{}, err
	}
	batch.Prompt = nullStringText(prompt)
	batch.Mood = nullStringText(mood)
	batch.Notes = nullStringText(notes)

	candidates, err := fetchRecommendationCandidates(ctx, db, id)
	if err != nil {
		return RecommendationBatch{}, err
	}
	batch.Candidates = candidates
	return batch, nil
}

func ListRecommendationBatches(ctx context.Context, db *sql.DB, limit int) ([]RecommendationBatchSummary, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := db.QueryContext(ctx, `
		SELECT b.id, b.prompt, b.mood, b.notes, b.created_at, (
			SELECT COUNT(*) FROM recommendation_candidates c WHERE c.batch_id = b.id
		)
		FROM recommendation_batches b
		ORDER BY b.created_at DESC, b.rowid DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []RecommendationBatchSummary
	for rows.Next() {
		var summary RecommendationBatchSummary
		var prompt, mood, notes, createdAt sql.NullString
		if err := rows.Scan(
			&summary.ID,
			&prompt,
			&mood,
			&notes,
			&createdAt,
			&summary.CandidateCount,
		); err != nil {
			return nil, err
		}
		summary.Prompt = nullStringText(prompt)
		summary.Mood = nullStringText(mood)
		summary.CreatedAt = nullStringText(createdAt)
		summary.Reply = nullStringText(notes)
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return summaries, nil
}

func fetchRecommendationCandidates(ctx context.Context, db *sql.DB, batchID string) ([]RecommendationCandidate, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, batch_id, artist, album, clean_artist, clean_title, starter_track, release_year, genre_tags, rank, streaming_url
		FROM recommendation_candidates
		WHERE batch_id = ?
		ORDER BY rank ASC, rowid ASC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []RecommendationCandidate
	for rows.Next() {
		var candidate RecommendationCandidate
		var starterTrack, genreTags, streamingURL sql.NullString
		var releaseYear sql.NullInt64
		if err := rows.Scan(
			&candidate.ID,
			&candidate.BatchID,
			&candidate.Artist,
			&candidate.Album,
			&candidate.CleanArtist,
			&candidate.CleanTitle,
			&starterTrack,
			&releaseYear,
			&genreTags,
			&candidate.Rank,
			&streamingURL,
		); err != nil {
			return nil, err
		}
		candidate.StarterTrack = nullStringText(starterTrack)
		candidate.StreamingURL = nullStringText(streamingURL)
		if releaseYear.Valid {
			candidate.ReleaseYear = int(releaseYear.Int64)
		}
		if genreTags.Valid && strings.TrimSpace(genreTags.String) != "" {
			if err := json.Unmarshal([]byte(genreTags.String), &candidate.GenreTags); err != nil {
				return nil, fmt.Errorf("unmarshal recommendation candidate genre tags: %w", err)
			}
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return candidates, nil
}

func CanonicalRecommendationVerdict(value string) (string, bool) {
	verdict := strings.ToLower(strings.TrimSpace(value))
	verdict = strings.ReplaceAll(verdict, "’", "'")
	verdict = strings.ReplaceAll(verdict, "-", "_")
	verdict = strings.Join(strings.Fields(verdict), " ")

	switch verdict {
	case "disliked", "dislike", "hate", "hated", "no":
		return "disliked", true
	case "not_for_me_today", "not for me today", "not today", "wrong mood", "not the mood", "not in the mood":
		return "not_for_me_today", true
	case "ok", "okay", "it's ok", "its ok", "it is ok", "meh", "fine":
		return "ok", true
	case "good", "dig it", "liked", "like":
		return "good", true
	case "great", "excellent", "love it", "loved it", "love", "loved":
		return "great", true
	case "already_know", "already know", "already knew", "familiar", "known":
		return "already_know", true
	default:
		return "", false
	}
}

func NormalizeAlbumLookup(artist, album string) (string, string, error) {
	cleanArtist, err := utils.NormalizeSearchText(artist)
	if err != nil {
		return "", "", err
	}
	cleanTitle, err := utils.NormalizeSearchText(album)
	if err != nil {
		return "", "", err
	}

	return cleanArtist, cleanTitle, nil
}

func normalizeRecommendationCandidate(batchID string, fallbackRank int, input RecommendationCandidateInput) (RecommendationCandidate, error) {
	cleanArtist, cleanTitle, err := NormalizeAlbumLookup(input.Artist, input.Album)
	if err != nil {
		return RecommendationCandidate{}, err
	}

	rank := input.Rank
	if rank <= 0 {
		rank = fallbackRank
	}

	return RecommendationCandidate{
		ID:           uuid.NewString(),
		BatchID:      batchID,
		Artist:       strings.TrimSpace(input.Artist),
		Album:        strings.TrimSpace(input.Album),
		CleanArtist:  cleanArtist,
		CleanTitle:   cleanTitle,
		StarterTrack: strings.TrimSpace(input.StarterTrack),
		ReleaseYear:  input.ReleaseYear,
		GenreTags:    compactGenreTags(input.GenreTags),
		Rank:         rank,
		Note:         strings.TrimSpace(input.Note),
		StreamingURL: strings.TrimSpace(input.StreamingURL),
	}, nil
}

func compactGenreTags(values []string) []string {
	seen := make(map[string]bool, len(values))
	var tags []string
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		tags = append(tags, value)
	}
	return tags
}

func nullableTrimmedString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullablePositiveInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullStringText(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
