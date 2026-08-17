package database

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestCreateRecommendationBatchPersistsCandidates(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	db, err := InitDB(filepath.Join(t.TempDir(), "recommendations.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	batch, err := CreateRecommendationBatch(context.Background(), db.Ctx, RecommendationBatchInput{
		Prompt: "weird hip hop",
		Mood:   "dense",
		Candidates: []RecommendationCandidateInput{
			{
				Artist:       "Dos Monos",
				Album:        "Dos City!!!",
				StarterTrack: "In 20XX",
				ReleaseYear:  2019,
				GenreTags:    []string{"Experimental Hip Hop", "experimental hip hop", "Jazz Rap"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecommendationBatch() error = %v", err)
	}
	if _, err := uuid.Parse(batch.ID); err != nil {
		t.Fatalf("batch ID is not a UUID: %q", batch.ID)
	}
	if len(batch.Candidates) != 1 {
		t.Fatalf("len(batch.Candidates) = %d, want 1", len(batch.Candidates))
	}
	candidate := batch.Candidates[0]
	if _, err := uuid.Parse(candidate.ID); err != nil {
		t.Fatalf("candidate ID is not a UUID: %q", candidate.ID)
	}
	if candidate.CleanArtist != "dos monos" || candidate.CleanTitle != "dos city" {
		t.Fatalf("clean candidate = %q/%q, want dos monos/dos city", candidate.CleanArtist, candidate.CleanTitle)
	}
	if len(candidate.GenreTags) != 2 || candidate.GenreTags[0] != "experimental hip hop" || candidate.GenreTags[1] != "jazz rap" {
		t.Fatalf("candidate.GenreTags = %#v, want deduplicated lowercase tags", candidate.GenreTags)
	}

	var count int
	if err := db.Ctx.QueryRow("SELECT COUNT(*) FROM recommendation_candidates WHERE batch_id = ?", batch.ID).Scan(&count); err != nil {
		t.Fatalf("failed to count persisted candidates: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted candidate count = %d, want 1", count)
	}
}

func TestLogRecommendationFeedbackCanonicalizesVerdict(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	db, err := InitDB(filepath.Join(t.TempDir(), "feedback.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	result, err := LogRecommendationFeedback(context.Background(), db.Ctx, RecommendationFeedbackInput{
		Artist:       "King Gizzard & The Lizard Wizard",
		Album:        "Alien Metal",
		StarterTrack: "Kill for the Steel",
		Verdict:      "love it",
		Notes:        "release day listen",
	})
	if err != nil {
		t.Fatalf("LogRecommendationFeedback() error = %v", err)
	}
	if result.Verdict != "great" {
		t.Fatalf("result.Verdict = %q, want great", result.Verdict)
	}
	if result.CleanArtist != "king gizzard and the lizard wizard" {
		t.Fatalf("result.CleanArtist = %q, want normalized King Gizzard", result.CleanArtist)
	}

	recent, err := FetchRecentRecommendationFeedback(context.Background(), db.Ctx, 5)
	if err != nil {
		t.Fatalf("FetchRecentRecommendationFeedback() error = %v", err)
	}
	if len(recent) != 1 || recent[0].Album != "Alien Metal" || recent[0].Verdict != "great" {
		t.Fatalf("recent feedback = %#v, want Alien Metal great", recent)
	}
}
