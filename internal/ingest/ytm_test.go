package ingest

import (
	"path/filepath"
	"testing"

	"github.com/nicksunday/music-context-platform/internal/database"
)

func TestIngestYTMLibraryPopulatesCleanColumns(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	tempDir := t.TempDir()
	db, err := database.InitDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	library := filepath.Join(tempDir, "music library songs.csv")
	writeTestCSV(t, library, `Playlist,Title,Album,Artist
"Favorites","Canción","Café","Beyoncé"
`)

	if err := IngestYTMLibrary(db, library); err != nil {
		t.Fatalf("IngestYTMLibrary() error = %v", err)
	}

	var cleanTitle, cleanArtist string
	err = db.Ctx.QueryRow(
		"SELECT clean_title, clean_artist FROM tracks WHERE title = ?",
		"Canción",
	).Scan(&cleanTitle, &cleanArtist)
	if err != nil {
		t.Fatalf("failed to query track clean columns: %v", err)
	}

	if cleanTitle != "cancion" {
		t.Fatalf("cleanTitle = %q, want %q", cleanTitle, "cancion")
	}
	if cleanArtist != "beyonce" {
		t.Fatalf("cleanArtist = %q, want %q", cleanArtist, "beyonce")
	}
}
