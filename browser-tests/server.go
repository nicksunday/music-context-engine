package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/nicksunday/music-context-platform/internal/database"
	"github.com/nicksunday/music-context-platform/internal/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18787", "HTTP address")
	dbPath := flag.String("db", "browser-tests/.tmp/browser.db", "SQLite database path")
	flag.Parse()
	if err := os.Remove(*dbPath); err != nil && !os.IsNotExist(err) {
		log.Fatal(err)
	}
	db, err := database.InitDBAtPath(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Ctx.Close()
	seed(db)
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("browser test server listening on %s", *addr)
	if err := http.Serve(listener, web.NewServer(db.Ctx, web.Options{})); err != nil {
		log.Fatal(err)
	}
}

func seed(db *database.DBClient) {
	album, err := database.CreateRecommendationBatch(context.Background(), db.Ctx, database.RecommendationBatchInput{Prompt: "album request", Mode: "album", Candidates: []database.RecommendationCandidateInput{{Artist: "Browser Artist", Album: "Browser Album", StarterTrack: "Browser Track"}}})
	if err != nil {
		log.Fatal(err)
	}
	song, err := database.CreateRecommendationBatch(context.Background(), db.Ctx, database.RecommendationBatchInput{Prompt: "song request", Mode: "song", Candidates: []database.RecommendationCandidateInput{{Artist: "Browser Artist", Album: "Browser Album", Song: "Browser Song"}}})
	if err != nil {
		log.Fatal(err)
	}
	if _, err := database.SaveRecommendationPromptFitFeedback(context.Background(), db.Ctx, database.RecommendationPromptFitFeedbackInput{BatchID: album.ID, CandidateID: album.Candidates[0].ID, Verdict: "met"}); err != nil {
		log.Fatal(err)
	}
	if _, err := database.SaveRecommendationPromptFitFeedback(context.Background(), db.Ctx, database.RecommendationPromptFitFeedbackInput{BatchID: song.ID, CandidateID: song.Candidates[0].ID, Verdict: "missed", Reason: "wrong_energy"}); err != nil {
		log.Fatal(err)
	}
}
