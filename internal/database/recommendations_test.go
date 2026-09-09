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
				StreamingURL: "  https://music.apple.com/us/album/dos-city/1450632733  ",
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
	const wantStreamingURL = "https://music.apple.com/us/album/dos-city/1450632733"
	if candidate.StreamingURL != wantStreamingURL {
		t.Fatalf("candidate.StreamingURL = %q, want %q", candidate.StreamingURL, wantStreamingURL)
	}

	var count int
	if err := db.Ctx.QueryRow("SELECT COUNT(*) FROM recommendation_candidates WHERE batch_id = ?", batch.ID).Scan(&count); err != nil {
		t.Fatalf("failed to count persisted candidates: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted candidate count = %d, want 1", count)
	}

	var persistedStreamingURL string
	if err := db.Ctx.QueryRow("SELECT streaming_url FROM recommendation_candidates WHERE batch_id = ?", batch.ID).Scan(&persistedStreamingURL); err != nil {
		t.Fatalf("failed to query persisted streaming_url: %v", err)
	}
	if persistedStreamingURL != wantStreamingURL {
		t.Fatalf("persisted streaming_url = %q, want %q", persistedStreamingURL, wantStreamingURL)
	}
}

func TestRecommendationBatchPersistsSongAndProviderFields(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	db, err := InitDB(filepath.Join(t.TempDir(), "song-recommendations.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	created, err := CreateRecommendationBatch(context.Background(), db.Ctx, RecommendationBatchInput{
		Prompt: "individual songs with intricate rhythm",
		Candidates: []RecommendationCandidateInput{{
			Artist:            "Jungle",
			Album:             "Volcano",
			Song:              "Candle Flame",
			StreamingURL:      "https://music.apple.com/us/song/candle-flame/1",
			StreamingProvider: "apple_music",
		}},
	})
	if err != nil {
		t.Fatalf("CreateRecommendationBatch() error = %v", err)
	}
	if got := created.Candidates[0].Song; got != "Candle Flame" {
		t.Fatalf("created candidate song = %q, want Candle Flame", got)
	}
	if got := created.Candidates[0].StreamingProvider; got != "apple_music" {
		t.Fatalf("created candidate provider = %q, want apple_music", got)
	}

	loaded, err := FetchRecommendationBatchByID(context.Background(), db.Ctx, created.ID)
	if err != nil {
		t.Fatalf("FetchRecommendationBatchByID() error = %v", err)
	}
	if got := loaded.Candidates[0].Song; got != "Candle Flame" {
		t.Fatalf("loaded candidate song = %q, want Candle Flame", got)
	}
	if got := loaded.Candidates[0].StreamingProvider; got != "apple_music" {
		t.Fatalf("loaded candidate provider = %q, want apple_music", got)
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

func TestFetchRecentRecommendationFeedbackResolvesCandidateGenreTags(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)
	db, err := InitDB(filepath.Join(t.TempDir(), "feedback-genres.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Ctx.Close()

	batch, err := CreateRecommendationBatch(context.Background(), db.Ctx, RecommendationBatchInput{
		Prompt: "progressive metal",
		Candidates: []RecommendationCandidateInput{{
			Artist: "Test Artist", Album: "Test Album", GenreTags: []string{"Progressive Metal", "progressive metal", "Power Metal"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = LogRecommendationFeedback(context.Background(), db.Ctx, RecommendationFeedbackInput{
		Artist: "Test Artist", Album: "Test Album", CandidateID: batch.Candidates[0].ID, Verdict: "good",
	})
	if err != nil {
		t.Fatal(err)
	}

	feedback, err := FetchRecentRecommendationFeedback(context.Background(), db.Ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(feedback) != 1 || len(feedback[0].GenreTags) != 2 || feedback[0].GenreTags[0] != "progressive metal" || feedback[0].GenreTags[1] != "power metal" {
		t.Fatalf("feedback = %#v, want normalized candidate genre tags", feedback)
	}
}

func TestFetchRecentRecommendationFeedbackFallsBackToLocalAlbumGenres(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)
	db, err := InitDB(filepath.Join(t.TempDir(), "feedback-local-genres.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Ctx.Close()

	_, err = db.Ctx.Exec(`INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres) VALUES (?, ?, ?, ?, ?, ?)`,
		"local-album", "Local Album", "Local Artist", "local album", "local artist", `["Art Rock","Progressive Rock"]`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LogRecommendationFeedback(context.Background(), db.Ctx, RecommendationFeedbackInput{
		Artist: "Local Artist", Album: "Local Album", Verdict: "great",
	}); err != nil {
		t.Fatal(err)
	}

	feedback, err := FetchRecentRecommendationFeedback(context.Background(), db.Ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(feedback) != 1 || len(feedback[0].GenreTags) != 2 || feedback[0].GenreTags[0] != "art rock" || feedback[0].GenreTags[1] != "progressive rock" {
		t.Fatalf("feedback = %#v, want local album genre fallback", feedback)
	}
}

func TestFetchLatestRecommendationBatchEmptyWhenNoBatches(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	db, err := InitDB(filepath.Join(t.TempDir(), "empty-recommendations.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	batch, err := FetchLatestRecommendationBatch(context.Background(), db.Ctx)
	if err != nil {
		t.Fatalf("FetchLatestRecommendationBatch() error = %v", err)
	}
	if batch.ID != "" || len(batch.Candidates) != 0 {
		t.Fatalf("FetchLatestRecommendationBatch() = %#v, want empty batch", batch)
	}
}

func TestFetchAndListRecommendationBatches(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	db, err := InitDB(filepath.Join(t.TempDir(), "history-recommendations.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	first, err := CreateRecommendationBatch(context.Background(), db.Ctx, RecommendationBatchInput{
		Prompt: "dense jazz",
		Mood:   "focused",
		Candidates: []RecommendationCandidateInput{
			{Artist: "Bohren & der Club of Gore", Album: "Sunset Mission", StarterTrack: "Midnight Walker", StreamingURL: "https://music.apple.com/us/album/sunset-mission/1"},
			{Artist: "Second Artist", Album: "Second Album", StarterTrack: "Second Track"},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecommendationBatch(first) error = %v", err)
	}
	if len(first.Candidates) != 2 {
		t.Fatalf("first batch candidates = %d, want 2", len(first.Candidates))
	}

	second, err := CreateRecommendationBatch(context.Background(), db.Ctx, RecommendationBatchInput{
		Prompt: "weird hip hop",
		Mood:   "dense",
		Candidates: []RecommendationCandidateInput{
			{Artist: "Jungle", Album: "Volcano", StarterTrack: "Candle Flame", StreamingURL: "https://music.apple.com/us/album/volcano/2"},
		},
	})
	if err != nil {
		t.Fatalf("CreateRecommendationBatch(second) error = %v", err)
	}
	if len(second.Candidates) != 1 {
		t.Fatalf("second batch candidates = %d, want 1", len(second.Candidates))
	}

	latest, err := FetchLatestRecommendationBatch(context.Background(), db.Ctx)
	if err != nil {
		t.Fatalf("FetchLatestRecommendationBatch() error = %v", err)
	}
	if latest.ID != second.ID {
		t.Fatalf("latest batch id = %q, want %q", latest.ID, second.ID)
	}
	if len(latest.Candidates) != 1 || latest.Candidates[0].Album != "Volcano" {
		t.Fatalf("latest candidates = %#v, want Volcano", latest.Candidates)
	}
	if latest.Candidates[0].StreamingURL != "https://music.apple.com/us/album/volcano/2" {
		t.Fatalf("latest candidate streaming_url = %q, want volcano url", latest.Candidates[0].StreamingURL)
	}

	byID, err := FetchRecommendationBatchByID(context.Background(), db.Ctx, first.ID)
	if err != nil {
		t.Fatalf("FetchRecommendationBatchByID() error = %v", err)
	}
	if len(byID.Candidates) != 2 {
		t.Fatalf("first batch by id = %#v, want 2 candidates", byID.Candidates)
	}
	if byID.Candidates[0].StreamingURL != "https://music.apple.com/us/album/sunset-mission/1" {
		t.Fatalf("first candidate streaming_url = %q, want restored url", byID.Candidates[0].StreamingURL)
	}

	sessions, err := ListRecommendationBatches(context.Background(), db.Ctx, 10)
	if err != nil {
		t.Fatalf("ListRecommendationBatches() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("session count = %d, want 2", len(sessions))
	}
	if sessions[0].ID != second.ID || sessions[0].CandidateCount != 1 {
		t.Fatalf("sessions[0] = %#v, want newest batch with 1 candidate", sessions[0])
	}
	if sessions[1].ID != first.ID || sessions[1].CandidateCount != 2 {
		t.Fatalf("sessions[1] = %#v, want older batch with 2 candidates", sessions[1])
	}
	if sessions[0].Prompt != "weird hip hop" || sessions[1].Prompt != "dense jazz" {
		t.Fatalf("session prompts out of order: %#v", sessions)
	}
}

func TestRecommendationBatchesAreIsolatedByMode(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)
	db, err := InitDB(filepath.Join(t.TempDir(), "mode-recommendations.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Ctx.Close()

	album, err := CreateRecommendationBatch(context.Background(), db.Ctx, RecommendationBatchInput{
		Prompt: "album prompt", Mode: "album", Candidates: []RecommendationCandidateInput{{Artist: "Album Artist", Album: "Album"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	song, err := CreateRecommendationBatch(context.Background(), db.Ctx, RecommendationBatchInput{
		Prompt: "song prompt", Mode: "song", Candidates: []RecommendationCandidateInput{{Artist: "Song Artist", Album: "Album", Song: "Song"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	latestSong, err := FetchLatestRecommendationBatchForMode(context.Background(), db.Ctx, "song")
	if err != nil || latestSong.ID != song.ID {
		t.Fatalf("latest song = %#v, err = %v", latestSong, err)
	}
	latestAlbum, err := FetchLatestRecommendationBatchForMode(context.Background(), db.Ctx, "album")
	if err != nil || latestAlbum.ID != album.ID {
		t.Fatalf("latest album = %#v, err = %v", latestAlbum, err)
	}

	songSessions, err := ListRecommendationBatchesForMode(context.Background(), db.Ctx, 10, "song")
	if err != nil || len(songSessions) != 1 || songSessions[0].Mode != "song" {
		t.Fatalf("song sessions = %#v, err = %v", songSessions, err)
	}
	legacy, err := CreateRecommendationBatch(context.Background(), db.Ctx, RecommendationBatchInput{
		Prompt: "legacy prompt", Candidates: []RecommendationCandidateInput{{Artist: "Legacy Artist", Album: "Legacy Album"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyAlbum, err := FetchLatestRecommendationBatchForMode(context.Background(), db.Ctx, "album")
	if err != nil || legacyAlbum.ID != legacy.ID {
		t.Fatalf("legacy album = %#v, err = %v", legacyAlbum, err)
	}
}
