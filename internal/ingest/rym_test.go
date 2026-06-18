package ingest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nicksunday/music-context-platform/internal/database"
)

func TestIngestRYMExportUpsertsAlbum(t *testing.T) {
	unsetMusicVaultDBPathEnv(t)

	tempDir := t.TempDir()
	db, err := database.InitDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	defer db.Ctx.Close()

	firstExport := filepath.Join(tempDir, "rym-music-export.csv")
	writeTestCSV(t, firstExport, `RYM Album ID, First Name , Last Name , Title , Release_Date , Rating
"1"," John "," Coltrane "," A Love Supreme ","1965","10"
`)

	if err := IngestRYMExport(db, firstExport); err != nil {
		t.Fatalf("IngestRYMExport() error = %v", err)
	}

	var title, artist, cleanTitle, cleanArtist string
	var rating float64
	var releaseDate int
	albumID := "1"
	err = db.Ctx.QueryRow(
		"SELECT title, artist, clean_title, clean_artist, user_rating, release_date FROM albums WHERE id = ?",
		albumID,
	).Scan(&title, &artist, &cleanTitle, &cleanArtist, &rating, &releaseDate)
	if err != nil {
		t.Fatalf("failed to query album: %v", err)
	}

	if title != "A Love Supreme" {
		t.Fatalf("title = %q, want %q", title, "A Love Supreme")
	}
	if artist != "John Coltrane" {
		t.Fatalf("artist = %q, want %q", artist, "John Coltrane")
	}
	if cleanTitle != "a love supreme" {
		t.Fatalf("cleanTitle = %q, want %q", cleanTitle, "a love supreme")
	}
	if cleanArtist != "john coltrane" {
		t.Fatalf("cleanArtist = %q, want %q", cleanArtist, "john coltrane")
	}
	if rating != 5.0 {
		t.Fatalf("rating = %v, want 5.0", rating)
	}
	if releaseDate != 1965 {
		t.Fatalf("releaseDate = %v, want 1965", releaseDate)
	}

	secondExport := filepath.Join(tempDir, "rym-music-export-2.csv")
	writeTestCSV(t, secondExport, `RYM Album ID, First Name,Last Name,Title,Release_Date,Rating
"1","John","Coltrane","A Love Supreme","1966","8"
`)

	if err := IngestRYMExport(db, secondExport); err != nil {
		t.Fatalf("second IngestRYMExport() error = %v", err)
	}

	var count int
	err = db.Ctx.QueryRow("SELECT COUNT(*) FROM albums WHERE id = ?", albumID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count albums: %v", err)
	}
	if count != 1 {
		t.Fatalf("album count = %d, want 1", count)
	}

	err = db.Ctx.QueryRow(
		"SELECT user_rating, release_date FROM albums WHERE id = ?",
		albumID,
	).Scan(&rating, &releaseDate)
	if err != nil {
		t.Fatalf("failed to query updated album: %v", err)
	}
	if rating != 4.0 {
		t.Fatalf("updated rating = %v, want 4.0", rating)
	}
	if releaseDate != 1966 {
		t.Fatalf("updated releaseDate = %v, want 1966", releaseDate)
	}
}

func writeTestCSV(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}
}

func unsetMusicVaultDBPathEnv(t *testing.T) {
	t.Helper()

	value, exists := os.LookupEnv(database.MusicVaultDBPathEnv)
	if err := os.Unsetenv(database.MusicVaultDBPathEnv); err != nil {
		t.Fatalf("failed to unset %s: %v", database.MusicVaultDBPathEnv, err)
	}
	t.Cleanup(func() {
		if exists {
			os.Setenv(database.MusicVaultDBPathEnv, value)
		} else {
			os.Unsetenv(database.MusicVaultDBPathEnv)
		}
	})
}
