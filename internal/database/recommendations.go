package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/nicksunday/music-context-platform/internal/utils"
)

type RecommendationCandidateInput struct {
	Artist            string   `json:"artist"`
	Album             string   `json:"album"`
	Song              string   `json:"song,omitempty"`
	StarterTrack      string   `json:"starter_track,omitempty"`
	ReleaseYear       int      `json:"release_year,omitempty"`
	GenreTags         []string `json:"genre_tags,omitempty"`
	Rank              int      `json:"rank,omitempty"`
	Note              string   `json:"note,omitempty"`
	StreamingURL      string   `json:"streaming_url,omitempty"`
	StreamingProvider string   `json:"streaming_provider,omitempty"`
	StreamingAppURL   string   `json:"streaming_app_url,omitempty"`
}

type RecommendationCandidate struct {
	ID                string                           `json:"id"`
	BatchID           string                           `json:"batch_id"`
	Artist            string                           `json:"artist"`
	Album             string                           `json:"album"`
	Song              string                           `json:"song,omitempty"`
	CleanArtist       string                           `json:"clean_artist"`
	CleanTitle        string                           `json:"clean_title"`
	StarterTrack      string                           `json:"starter_track,omitempty"`
	ReleaseYear       int                              `json:"release_year,omitempty"`
	GenreTags         []string                         `json:"genre_tags,omitempty"`
	Rank              int                              `json:"rank,omitempty"`
	Note              string                           `json:"note,omitempty"`
	StreamingURL      string                           `json:"streaming_url,omitempty"`
	StreamingProvider string                           `json:"streaming_provider,omitempty"`
	StreamingAppURL   string                           `json:"streaming_app_url,omitempty"`
	PromptFit         *RecommendationPromptFitFeedback `json:"prompt_fit,omitempty"`
}

type RecommendationBatchInput struct {
	Prompt     string                            `json:"prompt"`
	Mood       string                            `json:"mood,omitempty"`
	Notes      string                            `json:"notes,omitempty"`
	Mode       string                            `json:"mode,omitempty"`
	Candidates []RecommendationCandidateInput    `json:"candidates"`
	Snapshot   *RecommendationBatchSnapshotInput `json:"snapshot,omitempty"`
}

type RecommendationBatchSnapshotInput struct {
	SchemaVersion int            `json:"schema_version"`
	Payload       map[string]any `json:"payload"`
	Complete      bool           `json:"complete"`
}

type RecommendationPromptFitFeedbackInput struct {
	BatchID     string `json:"batch_id"`
	CandidateID string `json:"candidate_id"`
	Verdict     string `json:"verdict"`
	Reason      string `json:"reason,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

type RecommendationPromptFitFeedback struct {
	ID          string `json:"id"`
	BatchID     string `json:"batch_id"`
	CandidateID string `json:"candidate_id"`
	Verdict     string `json:"verdict"`
	Reason      string `json:"reason,omitempty"`
	Notes       string `json:"notes,omitempty"`
	Revision    int    `json:"revision"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type RecommendationExample struct {
	ID           string `json:"id"`
	RequestKey   string `json:"request_key"`
	EntityScope  string `json:"entity_scope"`
	SuppliedText string `json:"supplied_text"`
	Artist       string `json:"artist,omitempty"`
	Album        string `json:"album,omitempty"`
	Song         string `json:"song,omitempty"`
	Polarity     string `json:"polarity"`
	Notes        string `json:"notes,omitempty"`
	Revision     int    `json:"revision"`
	Active       bool   `json:"active"`
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

func RecommendationRequestKey(message, mood, avoid, mode string) string {
	value := strings.Join([]string{strings.TrimSpace(message), strings.TrimSpace(mood), strings.TrimSpace(avoid), normalizedRecommendationMode(mode)}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:])
}

type RecommendationExportDraft struct {
	ID             string `json:"id"`
	FeedbackID     string `json:"feedback_id"`
	Payload        string `json:"payload"`
	SourceRevision int    `json:"source_revision"`
	ContentHash    string `json:"content_hash"`
	Approved       bool   `json:"approved"`
}

type RecommendationExportReviewItem struct {
	FeedbackID         string `json:"feedback_id"`
	BatchID            string `json:"batch_id"`
	CandidateID        string `json:"candidate_id"`
	Request            string `json:"request"`
	Mode               string `json:"mode"`
	Verdict            string `json:"verdict"`
	Reason             string `json:"reason,omitempty"`
	Notes              string `json:"notes,omitempty"`
	Revision           int    `json:"revision"`
	ProvenanceComplete bool   `json:"provenance_complete"`
	DraftID            string `json:"draft_id,omitempty"`
	Approved           bool   `json:"approved"`
}

func ListRecommendationExportReview(ctx context.Context, db *sql.DB, limit int) ([]RecommendationExportReviewItem, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `
		SELECT f.id, f.batch_id, f.candidate_id, b.prompt, COALESCE(b.mode,'album'), f.verdict,
			COALESCE(f.reason,''), COALESCE(f.notes,''), f.revision,
			CASE WHEN s.batch_id IS NOT NULL AND s.complete = 1 THEN 1 ELSE 0 END,
			COALESCE(d.id,''), CASE WHEN d.approved_at IS NOT NULL AND d.source_revision = f.revision THEN 1 ELSE 0 END
		FROM recommendation_prompt_fit_feedback f
		JOIN recommendation_batches b ON b.id=f.batch_id
		LEFT JOIN recommendation_batch_snapshots s ON s.batch_id=f.batch_id
		LEFT JOIN recommendation_export_drafts d ON d.feedback_id=f.id
		ORDER BY f.updated_at DESC, f.rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []RecommendationExportReviewItem
	for rows.Next() {
		var item RecommendationExportReviewItem
		var complete, approved int
		if err := rows.Scan(&item.FeedbackID, &item.BatchID, &item.CandidateID, &item.Request, &item.Mode, &item.Verdict, &item.Reason, &item.Notes, &item.Revision, &complete, &item.DraftID, &approved); err != nil {
			return nil, err
		}
		item.ProvenanceComplete, item.Approved = complete != 0, approved != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

type RecommendationExportExample struct {
	SchemaVersion      int                             `json:"schema_version"`
	ExampleID          string                          `json:"example_id"`
	Mode               string                          `json:"mode"`
	Request            string                          `json:"request"`
	Candidate          PortableRecommendationCandidate `json:"candidate"`
	Fit                PortablePromptFit               `json:"fit"`
	ProvenanceComplete bool                            `json:"provenance_complete"`
	ReplayEligible     bool                            `json:"replay_eligible"`
}

type PortablePromptFit struct {
	Verdict string `json:"verdict"`
	Reason  string `json:"reason,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

type PortableRecommendationCandidate struct {
	Artist       string   `json:"artist"`
	Album        string   `json:"album"`
	Song         string   `json:"song,omitempty"`
	StarterTrack string   `json:"starter_track,omitempty"`
	ReleaseYear  int      `json:"release_year,omitempty"`
	GenreTags    []string `json:"genre_tags,omitempty"`
	Rank         int      `json:"rank,omitempty"`
	Note         string   `json:"note,omitempty"`
	StreamingURL string   `json:"streaming_url,omitempty"`
}

func BuildRecommendationExportExample(ctx context.Context, db *sql.DB, feedbackID string) (RecommendationExportExample, error) {
	var example RecommendationExportExample
	var batchID, prompt, mode string
	var candidateID string
	var fit RecommendationPromptFitFeedback
	if err := db.QueryRowContext(ctx, `SELECT f.id, f.batch_id, f.candidate_id, f.verdict, COALESCE(f.reason,''), COALESCE(f.notes,''), f.revision, f.created_at, f.updated_at, b.prompt, COALESCE(b.mode,'album') FROM recommendation_prompt_fit_feedback f JOIN recommendation_batches b ON b.id=f.batch_id WHERE f.id=?`, feedbackID).Scan(&fit.ID, &batchID, &candidateID, &fit.Verdict, &fit.Reason, &fit.Notes, &fit.Revision, &fit.CreatedAt, &fit.UpdatedAt, &prompt, &mode); err != nil {
		return example, err
	}
	batch, err := FetchRecommendationBatchByID(ctx, db, batchID)
	if err != nil {
		return example, err
	}
	for _, candidate := range batch.Candidates {
		if candidate.ID == candidateID {
			example.Candidate = PortableRecommendationCandidate{
				Artist: candidate.Artist, Album: candidate.Album, Song: candidate.Song,
				StarterTrack: candidate.StarterTrack, ReleaseYear: candidate.ReleaseYear,
				GenreTags: append([]string(nil), candidate.GenreTags...), Rank: candidate.Rank,
				Note: candidate.Note, StreamingURL: candidate.StreamingURL,
			}
			break
		}
	}
	if example.Candidate.Artist == "" || example.Candidate.Album == "" {
		return example, fmt.Errorf("fit candidate not found")
	}
	example.SchemaVersion, example.ExampleID, example.Mode, example.Request = 1, PortableExampleID(feedbackID), mode, prompt
	example.Fit = PortablePromptFit{Verdict: fit.Verdict, Reason: fit.Reason, Notes: fit.Notes}
	example.ProvenanceComplete = batch.SnapshotAvailable && batch.Snapshot != nil && batch.Snapshot.Complete
	example.ReplayEligible = example.ProvenanceComplete
	return example, nil
}

func PortableExampleID(feedbackID string) string {
	return fmt.Sprintf("fit-example-%x", sha256.Sum256([]byte(strings.TrimSpace(feedbackID))))[:22]
}

func SaveRecommendationExportDraft(ctx context.Context, db *sql.DB, feedbackID, payload string) (RecommendationExportDraft, error) {
	feedbackID, payload = strings.TrimSpace(feedbackID), strings.TrimSpace(payload)
	if db == nil || feedbackID == "" || payload == "" {
		return RecommendationExportDraft{}, fmt.Errorf("feedback and draft payload are required")
	}
	var example RecommendationExportExample
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&example); err != nil || example.SchemaVersion != 1 || example.ExampleID == "" {
		return RecommendationExportDraft{}, fmt.Errorf("draft payload must be a valid JSONL example schema v1")
	}
	var sourceVerdict string
	if err := db.QueryRowContext(ctx, "SELECT verdict FROM recommendation_prompt_fit_feedback WHERE id = ?", feedbackID).Scan(&sourceVerdict); err != nil {
		return RecommendationExportDraft{}, err
	}
	if example.ExampleID != PortableExampleID(feedbackID) || example.Fit.Verdict != sourceVerdict {
		return RecommendationExportDraft{}, fmt.Errorf("draft payload does not match the selected source judgment")
	}
	var revision int
	if err := db.QueryRowContext(ctx, "SELECT revision FROM recommendation_prompt_fit_feedback WHERE id = ?", feedbackID).Scan(&revision); err != nil {
		return RecommendationExportDraft{}, err
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
	draft := RecommendationExportDraft{ID: uuid.NewString(), FeedbackID: feedbackID, Payload: payload, SourceRevision: revision, ContentHash: hash}
	var persistedID string
	err := db.QueryRowContext(ctx, `INSERT INTO recommendation_export_drafts (id, feedback_id, payload, source_revision, content_hash) VALUES (?, ?, ?, ?, ?) ON CONFLICT(feedback_id) DO UPDATE SET payload=excluded.payload, source_revision=excluded.source_revision, content_hash=excluded.content_hash, approved_at=NULL, updated_at=CURRENT_TIMESTAMP RETURNING id`, draft.ID, draft.FeedbackID, draft.Payload, draft.SourceRevision, draft.ContentHash).Scan(&persistedID)
	if err != nil {
		return RecommendationExportDraft{}, err
	}
	draft.ID = persistedID
	return draft, nil
}

func ApproveRecommendationExportDraft(ctx context.Context, db *sql.DB, id string) error {
	result, err := db.ExecContext(ctx, `UPDATE recommendation_export_drafts SET approved_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=? AND source_revision=(SELECT revision FROM recommendation_prompt_fit_feedback WHERE id=feedback_id)`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("draft is missing, stale, or source feedback was cleared")
	}
	return nil
}

func FetchApprovedRecommendationExportDrafts(ctx context.Context, db *sql.DB, ids []string) ([]RecommendationExportDraft, error) {
	var drafts []RecommendationExportDraft
	for _, id := range ids {
		var draft RecommendationExportDraft
		var approved sql.NullString
		var revision int
		if err := db.QueryRowContext(ctx, `SELECT d.id, d.feedback_id, d.payload, d.source_revision, d.content_hash, d.approved_at, f.revision FROM recommendation_export_drafts d JOIN recommendation_prompt_fit_feedback f ON f.id=d.feedback_id WHERE d.id=?`, strings.TrimSpace(id)).Scan(&draft.ID, &draft.FeedbackID, &draft.Payload, &draft.SourceRevision, &draft.ContentHash, &approved, &revision); err != nil {
			return nil, err
		}
		if !approved.Valid || revision != draft.SourceRevision || fmt.Sprintf("%x", sha256.Sum256([]byte(draft.Payload))) != draft.ContentHash {
			return nil, fmt.Errorf("draft %s is not currently approved", id)
		}
		draft.Approved = true
		drafts = append(drafts, draft)
	}
	return drafts, nil
}

type RecommendationBatch struct {
	ID                string                       `json:"id"`
	Prompt            string                       `json:"prompt"`
	Mood              string                       `json:"mood,omitempty"`
	Notes             string                       `json:"notes,omitempty"`
	Mode              string                       `json:"mode,omitempty"`
	Snapshot          *RecommendationBatchSnapshot `json:"snapshot,omitempty"`
	SnapshotAvailable bool                         `json:"snapshot_available"`
	Candidates        []RecommendationCandidate    `json:"candidates"`
}

type RecommendationBatchSummary struct {
	ID             string `json:"id"`
	Prompt         string `json:"prompt"`
	Mood           string `json:"mood,omitempty"`
	Mode           string `json:"mode,omitempty"`
	CreatedAt      string `json:"created_at"`
	CandidateCount int    `json:"candidate_count"`
	Reply          string `json:"reply,omitempty"`
}

type RecommendationBatchSnapshot struct {
	SchemaVersion int            `json:"schema_version"`
	Payload       map[string]any `json:"payload"`
	Complete      bool           `json:"complete"`
	CreatedAt     string         `json:"created_at,omitempty"`
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
	ID           string   `json:"id"`
	Artist       string   `json:"artist"`
	Album        string   `json:"album"`
	CleanArtist  string   `json:"clean_artist"`
	CleanTitle   string   `json:"clean_title"`
	StarterTrack string   `json:"starter_track,omitempty"`
	BatchID      string   `json:"batch_id,omitempty"`
	CandidateID  string   `json:"candidate_id,omitempty"`
	Verdict      string   `json:"verdict"`
	Mood         string   `json:"mood,omitempty"`
	Notes        string   `json:"notes,omitempty"`
	GenreTags    []string `json:"genre_tags,omitempty"`
	RowsInserted int64    `json:"rows_inserted,omitempty"`
	CreatedAt    string   `json:"created_at,omitempty"`
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
		Mode:   normalizedRecommendationMode(input.Mode),
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return RecommendationBatch{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO recommendation_batches (id, prompt, mood, notes, mode)
		VALUES (?, ?, ?, ?, ?)`,
		batch.ID,
		nullableTrimmedString(batch.Prompt),
		nullableTrimmedString(batch.Mood),
		nullableTrimmedString(batch.Notes),
		nullableTrimmedString(batch.Mode),
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
				id, batch_id, artist, album, song, clean_artist, clean_title, starter_track, release_year, genre_tags, rank, streaming_url, streaming_provider, streaming_app_url
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			candidate.ID,
			candidate.BatchID,
			candidate.Artist,
			candidate.Album,
			candidate.Song,
			candidate.CleanArtist,
			candidate.CleanTitle,
			nullableTrimmedString(candidate.StarterTrack),
			nullablePositiveInt(candidate.ReleaseYear),
			string(rawGenres),
			candidate.Rank,
			nullableTrimmedString(candidate.StreamingURL),
			nullableTrimmedString(candidate.StreamingProvider),
			nullableTrimmedString(candidate.StreamingAppURL),
		); err != nil {
			return RecommendationBatch{}, err
		}

		batch.Candidates = append(batch.Candidates, candidate)
	}
	if input.Snapshot != nil {
		if err := insertRecommendationBatchSnapshot(ctx, tx, batch.ID, *input.Snapshot); err != nil {
			return RecommendationBatch{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return RecommendationBatch{}, err
	}
	return batch, nil
}

func SaveRecommendationExample(ctx context.Context, db *sql.DB, input RecommendationExample) (RecommendationExample, error) {
	if db == nil {
		return RecommendationExample{}, fmt.Errorf("database is not initialized")
	}
	input.RequestKey = strings.TrimSpace(input.RequestKey)
	input.EntityScope = strings.ToLower(strings.TrimSpace(input.EntityScope))
	input.SuppliedText = strings.TrimSpace(input.SuppliedText)
	input.Artist = strings.TrimSpace(input.Artist)
	input.Album = strings.TrimSpace(input.Album)
	input.Song = strings.TrimSpace(input.Song)
	input.Polarity = strings.ToLower(strings.TrimSpace(input.Polarity))
	input.Notes = strings.TrimSpace(input.Notes)
	if input.RequestKey == "" || input.SuppliedText == "" {
		return RecommendationExample{}, fmt.Errorf("request_key and supplied_text are required")
	}
	if input.EntityScope != "artist" && input.EntityScope != "album" && input.EntityScope != "song" {
		return RecommendationExample{}, fmt.Errorf("invalid recommendation example entity scope")
	}
	if input.Polarity != "positive" && input.Polarity != "negative" {
		return RecommendationExample{}, fmt.Errorf("invalid recommendation example polarity")
	}
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO recommendation_examples (id, request_key, entity_scope, supplied_text, artist, album, song, polarity, notes, revision, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 1)
		ON CONFLICT(request_key, entity_scope, supplied_text, artist, album, song) DO UPDATE SET
			polarity=excluded.polarity, notes=excluded.notes, revision=recommendation_examples.revision+1,
			active=1, updated_at=CURRENT_TIMESTAMP`,
		input.ID, input.RequestKey, input.EntityScope, input.SuppliedText, input.Artist, input.Album, input.Song, input.Polarity, nullableTrimmedString(input.Notes))
	if err != nil {
		return RecommendationExample{}, err
	}
	return findRecommendationExample(ctx, db, input.RequestKey, input.EntityScope, input.SuppliedText, input.Artist, input.Album, input.Song)
}

func ListRecommendationExamples(ctx context.Context, db *sql.DB, requestKey string) ([]RecommendationExample, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, request_key, entity_scope, supplied_text, COALESCE(artist,''), COALESCE(album,''), COALESCE(song,''), polarity, COALESCE(notes,''), revision, active, created_at, updated_at FROM recommendation_examples WHERE request_key=? AND active=1 ORDER BY updated_at DESC, rowid DESC`, strings.TrimSpace(requestKey))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RecommendationExample
	for rows.Next() {
		var item RecommendationExample
		var active int
		if err := rows.Scan(&item.ID, &item.RequestKey, &item.EntityScope, &item.SuppliedText, &item.Artist, &item.Album, &item.Song, &item.Polarity, &item.Notes, &item.Revision, &active, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Active = active != 0
		result = append(result, item)
	}
	return result, rows.Err()
}

func ClearRecommendationExample(ctx context.Context, db *sql.DB, requestKey, entityScope, suppliedText, artist, album, song string) error {
	_, err := db.ExecContext(ctx, `UPDATE recommendation_examples SET active=0, revision=revision+1, updated_at=CURRENT_TIMESTAMP WHERE request_key=? AND entity_scope=? AND supplied_text=? AND COALESCE(artist,'')=? AND COALESCE(album,'')=? AND COALESCE(song,'')=?`, strings.TrimSpace(requestKey), strings.TrimSpace(entityScope), strings.TrimSpace(suppliedText), strings.TrimSpace(artist), strings.TrimSpace(album), strings.TrimSpace(song))
	return err
}

func findRecommendationExample(ctx context.Context, db *sql.DB, requestKey, scope, suppliedText, artist, album, song string) (RecommendationExample, error) {
	var item RecommendationExample
	var active int
	err := db.QueryRowContext(ctx, `SELECT id, request_key, entity_scope, supplied_text, COALESCE(artist,''), COALESCE(album,''), COALESCE(song,''), polarity, COALESCE(notes,''), revision, active, created_at, updated_at FROM recommendation_examples WHERE request_key=? AND entity_scope=? AND supplied_text=? AND COALESCE(artist,'')=? AND COALESCE(album,'')=? AND COALESCE(song,'')=?`, requestKey, scope, suppliedText, artist, album, song).Scan(&item.ID, &item.RequestKey, &item.EntityScope, &item.SuppliedText, &item.Artist, &item.Album, &item.Song, &item.Polarity, &item.Notes, &item.Revision, &active, &item.CreatedAt, &item.UpdatedAt)
	item.Active = active != 0
	return item, err
}

func insertRecommendationBatchSnapshot(ctx context.Context, tx *sql.Tx, batchID string, input RecommendationBatchSnapshotInput) error {
	version := input.SchemaVersion
	if version <= 0 {
		version = 1
	}
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return fmt.Errorf("marshal recommendation batch snapshot: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO recommendation_batch_snapshots (batch_id, schema_version, payload, complete)
		VALUES (?, ?, ?, ?)`, batchID, version, string(payload), boolInt(input.Complete))
	return err
}

func SaveRecommendationBatchSnapshot(ctx context.Context, db *sql.DB, batchID string, input RecommendationBatchSnapshotInput) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return fmt.Errorf("recommendation batch id must not be empty")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertRecommendationBatchSnapshot(ctx, tx, batchID, input); err != nil {
		return err
	}
	return tx.Commit()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

var recommendationPromptFitReasons = map[string]bool{
	"wrong_genre":            true,
	"wrong_energy":           true,
	"violated_exclusion":     true,
	"inaccurate_explanation": true,
	"other":                  true,
}

func CanonicalPromptFitVerdict(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "-", "_")))
	switch value {
	case "met", "met_my_request", "yes":
		return "met", true
	case "missed", "missed_my_request", "no":
		return "missed", true
	default:
		return "", false
	}
}

func SaveRecommendationPromptFitFeedback(ctx context.Context, db *sql.DB, input RecommendationPromptFitFeedbackInput) (RecommendationPromptFitFeedback, error) {
	if db == nil {
		return RecommendationPromptFitFeedback{}, fmt.Errorf("database is not initialized")
	}
	input.BatchID = strings.TrimSpace(input.BatchID)
	input.CandidateID = strings.TrimSpace(input.CandidateID)
	verdict, ok := CanonicalPromptFitVerdict(input.Verdict)
	if input.BatchID == "" || input.CandidateID == "" || !ok {
		return RecommendationPromptFitFeedback{}, fmt.Errorf("fit feedback requires a valid batch, candidate, and verdict")
	}
	reason := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(input.Reason, "-", "_")))
	if reason != "" && !recommendationPromptFitReasons[reason] {
		return RecommendationPromptFitFeedback{}, fmt.Errorf("unsupported prompt-fit reason %q", input.Reason)
	}
	var candidateBatch string
	if err := db.QueryRowContext(ctx, "SELECT batch_id FROM recommendation_candidates WHERE id = ?", input.CandidateID).Scan(&candidateBatch); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RecommendationPromptFitFeedback{}, fmt.Errorf("recommendation candidate not found")
		}
		return RecommendationPromptFitFeedback{}, err
	}
	if candidateBatch != input.BatchID {
		return RecommendationPromptFitFeedback{}, fmt.Errorf("candidate does not belong to recommendation batch")
	}

	result := RecommendationPromptFitFeedback{ID: uuid.NewString(), BatchID: input.BatchID, CandidateID: input.CandidateID, Verdict: verdict, Reason: reason, Notes: strings.TrimSpace(input.Notes)}
	err := db.QueryRowContext(ctx, `
		INSERT INTO recommendation_prompt_fit_feedback (id, batch_id, candidate_id, verdict, reason, notes)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(candidate_id) DO UPDATE SET verdict = excluded.verdict, reason = excluded.reason, notes = excluded.notes, revision = recommendation_prompt_fit_feedback.revision + 1, updated_at = CURRENT_TIMESTAMP
		RETURNING id, created_at, updated_at, revision`, result.ID, result.BatchID, result.CandidateID, result.Verdict, nullableTrimmedString(result.Reason), nullableTrimmedString(result.Notes)).Scan(&result.ID, &result.CreatedAt, &result.UpdatedAt, &result.Revision)
	if err != nil {
		return RecommendationPromptFitFeedback{}, err
	}
	result.BatchID = input.BatchID
	result.CandidateID = input.CandidateID
	result.Verdict = verdict
	result.Reason = reason
	result.Notes = strings.TrimSpace(input.Notes)
	return result, nil
}

func ClearRecommendationPromptFitFeedback(ctx context.Context, db *sql.DB, batchID, candidateID string) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	batchID, candidateID = strings.TrimSpace(batchID), strings.TrimSpace(candidateID)
	if batchID == "" || candidateID == "" {
		return fmt.Errorf("fit feedback requires a batch and candidate")
	}
	result, err := db.ExecContext(ctx, "DELETE FROM recommendation_prompt_fit_feedback WHERE batch_id = ? AND candidate_id = ?", batchID, candidateID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return nil
	}
	return nil
}

func FetchRecommendationBatchSnapshot(ctx context.Context, db *sql.DB, batchID string) (RecommendationBatchSnapshot, error) {
	var snapshot RecommendationBatchSnapshot
	var payload string
	var complete int
	err := db.QueryRowContext(ctx, `SELECT schema_version, payload, complete, created_at FROM recommendation_batch_snapshots WHERE batch_id = ?`, strings.TrimSpace(batchID)).Scan(&snapshot.SchemaVersion, &payload, &complete, &snapshot.CreatedAt)
	if err != nil {
		return snapshot, err
	}
	snapshot.Complete = complete != 0
	if err := json.Unmarshal([]byte(payload), &snapshot.Payload); err != nil {
		return RecommendationBatchSnapshot{}, fmt.Errorf("unmarshal recommendation batch snapshot: %w", err)
	}
	return snapshot, nil
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
		SELECT f.id, f.artist, f.album, f.clean_artist, f.clean_title, f.starter_track, f.batch_id, f.candidate_id, f.verdict, f.mood, f.notes, f.created_at,
			COALESCE(NULLIF(NULLIF(candidate.genre_tags, ''), '[]'), NULLIF(NULLIF(local_album.genres, ''), '[]'))
		FROM recommendation_feedback AS f
		LEFT JOIN recommendation_candidates AS candidate ON candidate.id = f.candidate_id
		LEFT JOIN albums AS local_album ON local_album.clean_artist = f.clean_artist AND local_album.clean_title = f.clean_title
		ORDER BY f.created_at DESC, f.rowid DESC
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
		var starterTrack, batchID, candidateID, mood, notes, createdAt, genreTags sql.NullString
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
			&genreTags,
		); err != nil {
			return nil, err
		}
		row.StarterTrack = nullStringText(starterTrack)
		row.BatchID = nullStringText(batchID)
		row.CandidateID = nullStringText(candidateID)
		row.Mood = nullStringText(mood)
		row.Notes = nullStringText(notes)
		row.CreatedAt = nullStringText(createdAt)
		if genreTags.Valid && strings.TrimSpace(genreTags.String) != "" && strings.TrimSpace(genreTags.String) != "[]" {
			if err := json.Unmarshal([]byte(genreTags.String), &row.GenreTags); err != nil {
				return nil, fmt.Errorf("unmarshal recommendation feedback genre tags: %w", err)
			}
			row.GenreTags = compactGenreTags(row.GenreTags)
		}
		feedback = append(feedback, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return feedback, nil
}

func FetchLatestRecommendationBatch(ctx context.Context, db *sql.DB) (RecommendationBatch, error) {
	return FetchLatestRecommendationBatchForMode(ctx, db, "album")
}

func FetchLatestRecommendationBatchForMode(ctx context.Context, db *sql.DB, mode string) (RecommendationBatch, error) {
	if db == nil {
		return RecommendationBatch{}, fmt.Errorf("database is not initialized")
	}

	var id string
	mode = normalizedRecommendationMode(mode)
	err := db.QueryRowContext(ctx, `
		SELECT id FROM recommendation_batches
		WHERE COALESCE(NULLIF(mode, ''), 'album') = ?
		ORDER BY created_at DESC, rowid DESC
		LIMIT 1`, mode).Scan(&id)
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
	var prompt, mood, notes, mode, createdAt sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT id, prompt, mood, notes, mode, created_at FROM recommendation_batches WHERE id = ?`, id,
	).Scan(&batch.ID, &prompt, &mood, &notes, &mode, &createdAt)
	if err != nil {
		return RecommendationBatch{}, err
	}
	batch.Prompt = nullStringText(prompt)
	batch.Mood = nullStringText(mood)
	batch.Notes = nullStringText(notes)
	batch.Mode = nullStringText(mode)

	candidates, err := fetchRecommendationCandidates(ctx, db, id)
	if err != nil {
		return RecommendationBatch{}, err
	}
	batch.Candidates = candidates
	if snapshot, snapshotErr := FetchRecommendationBatchSnapshot(ctx, db, id); snapshotErr == nil {
		batch.Snapshot = &snapshot
		batch.SnapshotAvailable = true
	} else if !errors.Is(snapshotErr, sql.ErrNoRows) {
		return RecommendationBatch{}, snapshotErr
	}
	return batch, nil
}

func ListRecommendationBatches(ctx context.Context, db *sql.DB, limit int) ([]RecommendationBatchSummary, error) {
	return ListRecommendationBatchesForMode(ctx, db, limit, "album")
}

func ListRecommendationBatchesForMode(ctx context.Context, db *sql.DB, limit int, mode string) ([]RecommendationBatchSummary, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if limit <= 0 {
		limit = 20
	}
	mode = normalizedRecommendationMode(mode)

	rows, err := db.QueryContext(ctx, `
		SELECT b.id, b.prompt, b.mood, b.notes, b.mode, b.created_at, (
			SELECT COUNT(*) FROM recommendation_candidates c WHERE c.batch_id = b.id
		)
		FROM recommendation_batches b
		WHERE COALESCE(NULLIF(b.mode, ''), 'album') = ?
		ORDER BY b.created_at DESC, b.rowid DESC
		LIMIT ?`, mode, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []RecommendationBatchSummary
	for rows.Next() {
		var summary RecommendationBatchSummary
		var prompt, mood, notes, modeValue, createdAt sql.NullString
		if err := rows.Scan(
			&summary.ID,
			&prompt,
			&mood,
			&notes,
			&modeValue,
			&createdAt,
			&summary.CandidateCount,
		); err != nil {
			return nil, err
		}
		summary.Prompt = nullStringText(prompt)
		summary.Mood = nullStringText(mood)
		summary.Mode = normalizedRecommendationMode(nullStringText(modeValue))
		summary.CreatedAt = nullStringText(createdAt)
		summary.Reply = nullStringText(notes)
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return summaries, nil
}

func normalizedRecommendationMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "song") || strings.EqualFold(strings.TrimSpace(mode), "songs") {
		return "song"
	}
	return "album"
}

func fetchRecommendationCandidates(ctx context.Context, db *sql.DB, batchID string) ([]RecommendationCandidate, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.id, c.batch_id, c.artist, c.album, c.song, c.clean_artist, c.clean_title, c.starter_track, c.release_year, c.genre_tags, c.rank, c.streaming_url, c.streaming_provider, c.streaming_app_url,
			f.id, f.verdict, f.reason, f.notes, f.revision, f.created_at, f.updated_at
		FROM recommendation_candidates
		AS c LEFT JOIN recommendation_prompt_fit_feedback AS f ON f.candidate_id = c.id
		WHERE c.batch_id = ?
		ORDER BY c.rank ASC, c.rowid ASC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []RecommendationCandidate
	for rows.Next() {
		var candidate RecommendationCandidate
		var song, starterTrack, genreTags, streamingURL, streamingProvider, streamingAppURL sql.NullString
		var fitID, fitVerdict, fitReason, fitNotes, fitCreated, fitUpdated sql.NullString
		var fitRevision sql.NullInt64
		var releaseYear sql.NullInt64
		if err := rows.Scan(
			&candidate.ID,
			&candidate.BatchID,
			&candidate.Artist,
			&candidate.Album,
			&song,
			&candidate.CleanArtist,
			&candidate.CleanTitle,
			&starterTrack,
			&releaseYear,
			&genreTags,
			&candidate.Rank,
			&streamingURL,
			&streamingProvider,
			&streamingAppURL,
			&fitID,
			&fitVerdict,
			&fitReason,
			&fitNotes,
			&fitRevision,
			&fitCreated,
			&fitUpdated,
		); err != nil {
			return nil, err
		}
		candidate.StarterTrack = nullStringText(starterTrack)
		candidate.StreamingURL = nullStringText(streamingURL)
		candidate.Song = nullStringText(song)
		candidate.StreamingProvider = nullStringText(streamingProvider)
		candidate.StreamingAppURL = nullStringText(streamingAppURL)
		if fitID.Valid {
			candidate.PromptFit = &RecommendationPromptFitFeedback{ID: fitID.String, BatchID: batchID, CandidateID: candidate.ID, Verdict: fitVerdict.String, Reason: nullStringText(fitReason), Notes: nullStringText(fitNotes), Revision: int(fitRevision.Int64), CreatedAt: nullStringText(fitCreated), UpdatedAt: nullStringText(fitUpdated)}
		}
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
		ID:                uuid.NewString(),
		BatchID:           batchID,
		Artist:            strings.TrimSpace(input.Artist),
		Album:             strings.TrimSpace(input.Album),
		Song:              strings.TrimSpace(input.Song),
		CleanArtist:       cleanArtist,
		CleanTitle:        cleanTitle,
		StarterTrack:      strings.TrimSpace(input.StarterTrack),
		ReleaseYear:       input.ReleaseYear,
		GenreTags:         compactGenreTags(input.GenreTags),
		Rank:              rank,
		Note:              strings.TrimSpace(input.Note),
		StreamingURL:      strings.TrimSpace(input.StreamingURL),
		StreamingProvider: strings.TrimSpace(input.StreamingProvider),
		StreamingAppURL:   strings.TrimSpace(input.StreamingAppURL),
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
