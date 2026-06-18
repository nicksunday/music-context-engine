package mcp

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
	mcpsrv "github.com/mark3labs/mcp-go/server"
	"github.com/nicksunday/music-context-platform/internal/database"
)

func TestSearchLibraryTracksNormalizesQuery(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist, genres)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"track-1",
		"Halo",
		"I Am... Sasha Fierce",
		"Beyoncé",
		"halo",
		"beyonce",
		`["pop","r&b"]`,
	)
	if err != nil {
		t.Fatalf("failed to insert track: %v", err)
	}

	tracks, err := searchLibraryTracks(context.Background(), db.Ctx, "BEYONCÉ")
	if err != nil {
		t.Fatalf("searchLibraryTracks() error = %v", err)
	}

	if len(tracks) != 1 {
		t.Fatalf("len(tracks) = %d, want 1", len(tracks))
	}
	if tracks[0].artist != "Beyoncé" {
		t.Fatalf("tracks[0].artist = %q, want %q", tracks[0].artist, "Beyoncé")
	}
	if tracks[0].genresJSON != `["pop","r&b"]` {
		t.Fatalf("tracks[0].genresJSON = %q, want %q", tracks[0].genresJSON, `["pop","r&b"]`)
	}
}

func TestSearchLibraryTracksCapsAt100(t *testing.T) {
	db := openTestDB(t)

	for i := 0; i < 101; i++ {
		_, err := db.Ctx.Exec(`
			INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist)
			VALUES (?, ?, ?, ?, ?, ?)`,
			"track-cap-"+strconv.Itoa(i),
			"Match "+strconv.Itoa(i),
			"Album",
			"Artist",
			"match "+strconv.Itoa(i),
			"artist",
		)
		if err != nil {
			t.Fatalf("failed to insert track %d: %v", i, err)
		}
	}

	tracks, err := searchLibraryTracks(context.Background(), db.Ctx, "match")
	if err != nil {
		t.Fatalf("searchLibraryTracks() error = %v", err)
	}

	if len(tracks) != libraryTrackCap {
		t.Fatalf("len(tracks) = %d, want %d", len(tracks), libraryTrackCap)
	}
}

func TestFetchFavoriteTracksFiltersGenreAndLimit(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist, genres, is_favorite)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?, ?)`,
		"track-1",
		"Blood and Thunder",
		"Leviathan",
		"Mastodon",
		"blood and thunder",
		"mastodon",
		`["sludge metal","progressive metal"]`,
		1,
		"track-2",
		"Oblivion",
		"Crack the Skye",
		"Mastodon",
		"oblivion",
		"mastodon",
		`["progressive metal"]`,
		1,
		"track-3",
		"Teardrop",
		"Mezzanine",
		"Massive Attack",
		"teardrop",
		"massive attack",
		`["100% electronic"]`,
		1,
		"track-4",
		"Halo",
		"I Am... Sasha Fierce",
		"Beyoncé",
		"halo",
		"beyonce",
		`["pop"]`,
		0,
	)
	if err != nil {
		t.Fatalf("failed to insert favorite fixtures: %v", err)
	}

	tracks, err := fetchFavoriteTracks(context.Background(), db.Ctx, "metal", 1)
	if err != nil {
		t.Fatalf("fetchFavoriteTracks() error = %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("len(tracks) = %d, want 1", len(tracks))
	}
	if tracks[0].title != "Blood and Thunder" {
		t.Fatalf("tracks[0].title = %q, want %q", tracks[0].title, "Blood and Thunder")
	}

	tracks, err = fetchFavoriteTracks(context.Background(), db.Ctx, "%", 10)
	if err != nil {
		t.Fatalf("fetchFavoriteTracks() with wildcard genre error = %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("len(tracks) = %d, want 1", len(tracks))
	}
	if tracks[0].title != "Teardrop" {
		t.Fatalf("tracks[0].title = %q, want %q", tracks[0].title, "Teardrop")
	}
}

func TestFetchTopRatedAlbumsFiltersGenre(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres, user_rating)
		VALUES
			(?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?)`,
		"album-1",
		"Crack the Skye",
		"Mastodon",
		"crack the skye",
		"mastodon",
		`["Progressive Metal","sludge"]`,
		4.8,
		"album-2",
		"Heaven or Las Vegas",
		"Cocteau Twins",
		"heaven or las vegas",
		"cocteau twins",
		`["dream pop"]`,
		4.9,
		"album-3",
		"Filosofem",
		"Burzum",
		"filosofem",
		"burzum",
		`["black metal"]`,
		4.7,
		"album-4",
		"100th Window",
		"Massive Attack",
		"100th window",
		"massive attack",
		`["100% electronic"]`,
		4.6,
	)
	if err != nil {
		t.Fatalf("failed to insert albums: %v", err)
	}

	albums, err := fetchTopRatedAlbums(context.Background(), db.Ctx, 4.0, "METAL")
	if err != nil {
		t.Fatalf("fetchTopRatedAlbums() error = %v", err)
	}
	if len(albums) != 2 {
		t.Fatalf("len(albums) = %d, want 2", len(albums))
	}
	if albums[0].title != "Crack the Skye" {
		t.Fatalf("albums[0].title = %q, want %q", albums[0].title, "Crack the Skye")
	}
	if albums[1].title != "Filosofem" {
		t.Fatalf("albums[1].title = %q, want %q", albums[1].title, "Filosofem")
	}

	albums, err = fetchTopRatedAlbums(context.Background(), db.Ctx, 4.0, "%")
	if err != nil {
		t.Fatalf("fetchTopRatedAlbums() with wildcard genre error = %v", err)
	}
	if len(albums) != 1 {
		t.Fatalf("len(albums) = %d, want 1", len(albums))
	}
	if albums[0].title != "100th Window" {
		t.Fatalf("albums[0].title = %q, want %q", albums[0].title, "100th Window")
	}
}

func TestFetchGenreDistributionCountsAlbumAndTrackGenres(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres)
		VALUES (?, ?, ?, ?, ?, ?);
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist, genres)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"album-1",
		"Album",
		"Artist",
		"album",
		"artist",
		`["metal","sludge"]`,
		"track-1",
		"Track",
		"Album",
		"Artist",
		"track",
		"artist",
		`["metal","pop"]`,
	)
	if err != nil {
		t.Fatalf("failed to insert genre fixtures: %v", err)
	}

	distribution, err := fetchGenreDistribution(context.Background(), db.Ctx)
	if err != nil {
		t.Fatalf("fetchGenreDistribution() error = %v", err)
	}
	if len(distribution) != 3 {
		t.Fatalf("len(distribution) = %d, want 3", len(distribution))
	}
	if distribution[0].subgenre != "metal" || distribution[0].total != 2 {
		t.Fatalf("distribution[0] = %#v, want metal count 2", distribution[0])
	}

	got := formatGenreDistributionMarkdown(distribution)
	want := "| Subgenre | Total Occurrences |\n" +
		"| --- | ---: |\n" +
		"| metal | 2 |\n" +
		"| pop | 1 |\n" +
		"| sludge | 1 |"
	if got != want {
		t.Fatalf("formatGenreDistributionMarkdown() = %q, want %q", got, want)
	}
}

func TestNewServerRegistersAndRoutesSpecTools(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, genres, user_rating)
		VALUES
			(?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?);
		INSERT INTO tracks (id, album_id, title, album, artist, clean_title, clean_artist, genres, is_favorite)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"album-1",
		"Crack the Skye",
		"Mastodon",
		"crack the skye",
		"mastodon",
		`["Progressive Metal","sludge"]`,
		4.8,
		"album-2",
		"I Am... Sasha Fierce",
		"Beyoncé",
		"i am sasha fierce",
		"beyonce",
		`["pop","r&b"]`,
		3.5,
		"track-1",
		"album-1",
		"Oblivion",
		"Crack the Skye",
		"Mastodon",
		"oblivion",
		"mastodon",
		`["progressive metal"]`,
		1,
		"track-2",
		nil,
		"Blood and Thunder",
		"Leviathan",
		"Mastodon",
		"blood and thunder",
		"mastodon",
		`["sludge metal"]`,
		1,
		"track-3",
		"album-2",
		"Halo",
		"I Am... Sasha Fierce",
		"Beyoncé",
		"halo",
		"beyonce",
		`["pop"]`,
		0,
	)
	if err != nil {
		t.Fatalf("failed to insert MCP fixtures: %v", err)
	}

	mcpServer := NewServer(db.Ctx)
	tools := mcpServer.ListTools()
	wantToolNames := []string{
		getLibraryTracksToolName,
		getFavoriteTracksToolName,
		getTopRatedAlbumsToolName,
		getGenreDistributionToolName,
		getAlbumTracksToolName,
		getTasteAdjacenciesToolName,
		checkAlbumHistoryToolName,
		logAlbumRatingToolName,
	}
	if len(tools) != len(wantToolNames) {
		t.Fatalf("len(tools) = %d, want %d", len(tools), len(wantToolNames))
	}

	registeredTools := make([]mcpsrv.ServerTool, 0, len(wantToolNames))
	for _, name := range wantToolNames {
		tool, ok := tools[name]
		if !ok {
			t.Fatalf("registered tool %q not found", name)
		}
		registeredTools = append(registeredTools, *tool)
	}

	testServer, err := mcptest.NewServer(t, registeredTools...)
	if err != nil {
		t.Fatalf("failed to start MCP test server: %v", err)
	}
	defer testServer.Close()

	ctx := context.Background()
	assertToolResponseContains := func(toolName string, args map[string]any, want string) {
		t.Helper()

		result := callTool(t, ctx, testServer, toolName, args)
		if result.IsError {
			t.Fatalf("%s returned error: %s", toolName, toolResultText(t, result))
		}
		got := toolResultText(t, result)
		if !strings.Contains(got, want) {
			t.Fatalf("%s response = %q, want to contain %q", toolName, got, want)
		}
	}

	assertToolResponseContains(getLibraryTracksToolName, map[string]any{"query": "mastodon"}, "Blood and Thunder")
	assertToolResponseContains(getFavoriteTracksToolName, map[string]any{"genre": "sludge", "limit": 1}, "Blood and Thunder")
	assertToolResponseContains(getTopRatedAlbumsToolName, map[string]any{"min_rating": 4.5, "genre": "progressive"}, "Crack the Skye")
	assertToolResponseContains(getGenreDistributionToolName, nil, "| Subgenre | Total Occurrences |")
	assertToolResponseContains(getAlbumTracksToolName, map[string]any{"artist": "Mastodon", "album": "Crack the Skye"}, "Oblivion")
	assertToolResponseContains(getTasteAdjacenciesToolName, map[string]any{"seed_artists": []any{"Mastodon"}, "target_vibe": "erratic rhythm section"}, "Direct Adjacencies")
	assertToolResponseContains(checkAlbumHistoryToolName, map[string]any{"artist": "Mastodon", "album": "Crack the Skye"}, "4.8/5")
	assertToolResponseContains(logAlbumRatingToolName, map[string]any{"artist": "Beyoncé", "album": "I Am... Sasha Fierce", "rating": 4.2}, "Updated")
}

func TestMCPToolsReturnErrorsForMalformedArgumentTypes(t *testing.T) {
	db := openTestDB(t)
	mcpServer := NewServer(db.Ctx)
	tools := mcpServer.ListTools()

	testServer, err := mcptest.NewServer(t,
		*tools[getLibraryTracksToolName],
		*tools[getFavoriteTracksToolName],
		*tools[getTopRatedAlbumsToolName],
		*tools[getAlbumTracksToolName],
		*tools[getTasteAdjacenciesToolName],
		*tools[checkAlbumHistoryToolName],
		*tools[logAlbumRatingToolName],
	)
	if err != nil {
		t.Fatalf("failed to start MCP test server: %v", err)
	}
	defer testServer.Close()

	ctx := context.Background()
	tests := []struct {
		name        string
		toolName    string
		args        map[string]any
		wantMessage string
	}{
		{
			name:        "library query must be string",
			toolName:    getLibraryTracksToolName,
			args:        map[string]any{"query": 12},
			wantMessage: "query argument must be a string",
		},
		{
			name:        "favorite limit must be integer",
			toolName:    getFavoriteTracksToolName,
			args:        map[string]any{"limit": "10"},
			wantMessage: "limit argument must be an integer",
		},
		{
			name:        "top rated min rating must be number",
			toolName:    getTopRatedAlbumsToolName,
			args:        map[string]any{"min_rating": "4.5"},
			wantMessage: "min_rating argument must be a number",
		},
		{
			name:        "album artist must be string",
			toolName:    getAlbumTracksToolName,
			args:        map[string]any{"artist": 7, "album": "Crack the Skye"},
			wantMessage: "artist argument must be a string",
		},
		{
			name:        "taste adjacency seed artists must be array",
			toolName:    getTasteAdjacenciesToolName,
			args:        map[string]any{"seed_artists": "Mastodon"},
			wantMessage: "seed_artists argument must be an array of strings",
		},
		{
			name:        "history album must be string",
			toolName:    checkAlbumHistoryToolName,
			args:        map[string]any{"artist": "Mastodon", "album": 12},
			wantMessage: "album argument must be a string",
		},
		{
			name:        "log rating must be number",
			toolName:    logAlbumRatingToolName,
			args:        map[string]any{"artist": "Mastodon", "album": "Crack the Skye", "rating": "4.5"},
			wantMessage: "rating argument must be a number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callTool(t, ctx, testServer, tt.toolName, tt.args)
			if !result.IsError {
				t.Fatalf("result.IsError = false, want true; text = %q", toolResultText(t, result))
			}
			if got := toolResultText(t, result); !strings.Contains(got, tt.wantMessage) {
				t.Fatalf("toolResultText() = %q, want to contain %q", got, tt.wantMessage)
			}
		})
	}
}

func TestFetchAlbumTracksNormalizesExactAlbumLookup(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist)
		VALUES (?, ?, ?, ?, ?);
		INSERT INTO tracks (id, album_id, title, album, artist, clean_title, clean_artist, is_favorite)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?, ?),
			(?, ?, ?, ?, ?, ?, ?, ?)`,
		"album-1",
		"Crème Brûlée",
		"François Hardy",
		"creme brulee",
		"francois hardy",
		"track-1",
		"album-1",
		"Intro",
		"Crème Brûlée",
		"François Hardy",
		"intro",
		"francois hardy",
		0,
		"track-2",
		"album-1",
		"Finale",
		"Crème Brûlée",
		"François Hardy",
		"finale",
		"francois hardy",
		1,
		"track-3",
		nil,
		"Bonus",
		"Crème Brûlée",
		"François Hardy",
		"bonus",
		"francois hardy",
		1,
		"track-4",
		nil,
		"Elsewhere",
		"Other",
		"François Hardy",
		"elsewhere",
		"francois hardy",
		1,
	)
	if err != nil {
		t.Fatalf("failed to insert album tracks: %v", err)
	}

	tracks, err := fetchAlbumTracks(context.Background(), db.Ctx, "FRANÇOIS HARDY", " creme brulee ")
	if err != nil {
		t.Fatalf("fetchAlbumTracks() error = %v", err)
	}
	if len(tracks) != 3 {
		t.Fatalf("len(tracks) = %d, want 3", len(tracks))
	}
	if tracks[0].title != "Intro" {
		t.Fatalf("tracks[0].title = %q, want %q", tracks[0].title, "Intro")
	}
	if !tracks[1].isFavorite {
		t.Fatal("tracks[1].isFavorite = false, want true")
	}
	if tracks[2].title != "Bonus" {
		t.Fatalf("tracks[2].title = %q, want %q", tracks[2].title, "Bonus")
	}

	got := formatAlbumTracksMarkdown(tracks)
	want := "- **Intro** by *François Hardy*\n" +
		"- **Finale** by *François Hardy* ❤️\n" +
		"- **Bonus** by *François Hardy* ❤️"
	if got != want {
		t.Fatalf("formatAlbumTracksMarkdown() = %q, want %q", got, want)
	}
}

func TestCheckAlbumHistoryAppliesSpec07Normalization(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, user_rating)
		VALUES (?, ?, ?, ?, ?, ?);
		INSERT INTO tracks (id, title, album, artist, clean_title, clean_artist, is_favorite)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"album-1",
		"Omnium Gatherum",
		"King Gizzard and The Lizard Wizard",
		"omnium gatherum",
		"king gizzard and the lizard wizard",
		4.4,
		"track-1",
		"Gaia",
		"Omnium Gatherum",
		"King Gizzard and The Lizard Wizard",
		"gaia",
		"king gizzard and the lizard wizard",
		1,
	)
	if err != nil {
		t.Fatalf("failed to insert album history fixtures: %v", err)
	}

	history, err := checkAlbumHistory(context.Background(), db.Ctx, "KING GIZZARD & THE LIZARD WIZARD", "Omnium Gatherum!!!")
	if err != nil {
		t.Fatalf("checkAlbumHistory() error = %v", err)
	}

	if !history.albumFound {
		t.Fatal("history.albumFound = false, want true")
	}
	if history.cleanArtist != "king gizzard and the lizard wizard" {
		t.Fatalf("history.cleanArtist = %q, want king gizzard and the lizard wizard", history.cleanArtist)
	}
	if history.cleanTitle != "omnium gatherum" {
		t.Fatalf("history.cleanTitle = %q, want omnium gatherum", history.cleanTitle)
	}
	if !history.userRating.Valid || !floatClose(history.userRating.Float64, 4.4) {
		t.Fatalf("history.userRating = %#v, want 4.4", history.userRating)
	}
	if history.artistTrackCount != 1 || history.artistFavoriteCount != 1 {
		t.Fatalf("track footprint = %d/%d, want 1/1", history.artistTrackCount, history.artistFavoriteCount)
	}
}

func TestLogAlbumRatingUpdatesExistingAlbumWithSpec07CleanKey(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Ctx.Exec(`
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, user_rating)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"album-1",
		"Omnium Gatherum",
		"King Gizzard and The Lizard Wizard",
		"omnium gatherum",
		"king gizzard and the lizard wizard",
		4.1,
	)
	if err != nil {
		t.Fatalf("failed to insert album rating fixture: %v", err)
	}

	result, err := logAlbumRating(context.Background(), db.Ctx, "King Gizzard & The Lizard Wizard", "Omnium Gatherum!!!", 4.7)
	if err != nil {
		t.Fatalf("logAlbumRating() error = %v", err)
	}

	if result.inserted {
		t.Fatal("result.inserted = true, want false")
	}
	if result.rowsUpdated != 1 {
		t.Fatalf("result.rowsUpdated = %d, want 1", result.rowsUpdated)
	}

	var rating float64
	if err := db.Ctx.QueryRow("SELECT user_rating FROM albums WHERE id = ?", "album-1").Scan(&rating); err != nil {
		t.Fatalf("failed to query updated rating: %v", err)
	}
	if !floatClose(rating, 4.7) {
		t.Fatalf("rating = %v, want 4.7", rating)
	}
}

func TestLogAlbumRatingInsertsUUIDAlbumWithCleanTokens(t *testing.T) {
	db := openTestDB(t)

	result, err := logAlbumRating(context.Background(), db.Ctx, "A&B", "C/D!!!", 3.5)
	if err != nil {
		t.Fatalf("logAlbumRating() error = %v", err)
	}

	if !result.inserted {
		t.Fatal("result.inserted = false, want true")
	}
	if _, err := uuid.Parse(result.albumID); err != nil {
		t.Fatalf("inserted album ID is not a UUID: %q", result.albumID)
	}
	if result.cleanArtist != "a and b" {
		t.Fatalf("result.cleanArtist = %q, want a and b", result.cleanArtist)
	}
	if result.cleanTitle != "c d" {
		t.Fatalf("result.cleanTitle = %q, want c d", result.cleanTitle)
	}

	var (
		title       string
		artist      string
		cleanTitle  string
		cleanArtist string
		rating      float64
	)
	err = db.Ctx.QueryRow(`
		SELECT title, artist, clean_title, clean_artist, user_rating
		FROM albums
		WHERE id = ?`,
		result.albumID,
	).Scan(&title, &artist, &cleanTitle, &cleanArtist, &rating)
	if err != nil {
		t.Fatalf("failed to query inserted album: %v", err)
	}

	if title != "C/D!!!" || artist != "A&B" {
		t.Fatalf("inserted title/artist = %q/%q, want C/D!!!/A&B", title, artist)
	}
	if cleanTitle != "c d" || cleanArtist != "a and b" {
		t.Fatalf("inserted clean title/artist = %q/%q, want c d/a and b", cleanTitle, cleanArtist)
	}
	if !floatClose(rating, 3.5) {
		t.Fatalf("rating = %v, want 3.5", rating)
	}
}

func callTool(t *testing.T, ctx context.Context, server *mcptest.Server, name string, args map[string]any) *mcpsdk.CallToolResult {
	t.Helper()

	var request mcpsdk.CallToolRequest
	request.Params.Name = name
	request.Params.Arguments = args

	result, err := server.Client().CallTool(ctx, request)
	if err != nil {
		t.Fatalf("CallTool(%q) error = %v", name, err)
	}

	return result
}

func toolResultText(t *testing.T, result *mcpsdk.CallToolResult) string {
	t.Helper()

	var builder strings.Builder
	for _, content := range result.Content {
		text, ok := content.(mcpsdk.TextContent)
		if !ok {
			t.Fatalf("unsupported content type: %T", content)
		}
		builder.WriteString(text.Text)
	}

	return builder.String()
}

func openTestDB(t *testing.T) *database.DBClient {
	t.Helper()

	unsetMusicVaultDBPathEnv(t)

	tempDir := t.TempDir()
	db, err := database.InitDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		db.Ctx.Close()
	})

	return db
}

func floatClose(got, want float64) bool {
	return math.Abs(got-want) < 0.000001
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
