package mcp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nicksunday/music-context-platform/internal/database"
	"github.com/nicksunday/music-context-platform/internal/utils"
)

const (
	serverName                = "music-context-platform-server"
	serverVersion             = "1.0.0"
	defaultMinRating          = 4.0
	defaultMaxAlbums          = 50
	libraryTrackCap           = 100
	defaultFavoriteTrackLimit = 50

	getLibraryTracksToolName      = "get_library_tracks"
	getFavoriteTracksToolName     = "get_favorite_tracks"
	getTopRatedAlbumsToolName     = "get_top_rated_albums"
	getGenreDistributionToolName  = "get_genre_distribution"
	getAlbumTracksToolName        = "get_album_tracks"
	getTasteAdjacenciesToolName   = "get_taste_adjacencies"
	getVerifiedCandidatesToolName = "get_verified_discovery_candidates"
	logAlbumRatingToolName        = "log_album_rating"

	defaultTasteArtistLimit = 12
	defaultTasteGenreLimit  = 20

	recommendationToolInstructions = `Use the local affinity/topography data as grounding. Enforce the sourcing hierarchy when researching: Metal Depth -> Encyclopaedia Metallum (metal-archives.com) and Shreddit Release Tracker; Progressive/Rock/Fusion -> ProgArchives (progarchives.com) and Fecking Bahamas (feckingbahamas.com); Roots/Virtuosic Acoustic -> Bluegrass Today (bluegrasstoday.com) and No Depression (nodepression.com); Hip Hop/Rap/Production -> HipHopDX (hiphopdx.com), Passion of the Weiss (passionweiss.com), and Dead End Hip Hop (deadendhiphop.com); Experimental/Electronic/Avant-Garde -> The Quietus (thequietus.com) and Resident Advisor (residentadvisor.net); Historical Canon -> 1001 Albums You Must Hear Before You Die and community-curated variations. Cross-reference musician pedigree and prefer sonic topology overlap over broad commercial genres. Final recommendation output must be grouped under Direct Adjacencies and Cross-Genre Wildcards.

CRITICAL OUTPUT CONTRACT:
1. STRICT TWO-SENTENCE LIMIT: The structural breakdown for each track MUST be exactly two sentences long. No run-on sentences, semicolons, or excessive comma splices to bypass this limit.
2. ANTI-GASLIGHTING RULE: If your pre-training data lacks deep, explicit knowledge of a track's actual sonic arrangements, you are FORBIDDEN from inventing descriptions (e.g., fabricating guitar style, production credits, or vocal style).
3. KNOWLEDGE FALLBACK: For obscure tracks, pivot the two sentences strictly to verifiable historical context, such as: "Returned via canonical tags [X]. While exact tracking arrangements are outside local parameters, [Artist] emerged from the [Year] [Scene/Subgenre] movement, mirroring the structural timeline of your request."`
)

func NewServer(db *sql.DB) *server.MCPServer {
	return newServer(db, defaultDiscoverySource())
}

func newServer(db *sql.DB, discovery discoverySource) *server.MCPServer {
	s := server.NewMCPServer(
		serverName,
		serverVersion,
		server.WithToolCapabilities(false),
	)

	registerTools(s, db, discovery)

	return s
}

func ServeStdio(db *sql.DB) error {
	return server.ServeStdio(NewServer(db))
}

func registerTools(s *server.MCPServer, db *sql.DB, discovery discoverySource) {
	s.AddTool(
		mcpsdk.NewTool(getLibraryTracksToolName,
			mcpsdk.WithDescription("Search local library tracks by artist or title."),
			mcpsdk.WithString("query",
				mcpsdk.Required(),
				mcpsdk.Description("Fragment to match against artist names or track titles."),
			),
		),
		getLibraryTracksHandler(db),
	)

	s.AddTool(
		mcpsdk.NewTool(getFavoriteTracksToolName,
			mcpsdk.WithDescription("Retrieve explicitly favorited tracks, optionally filtered by genre."),
			mcpsdk.WithString("genre",
				mcpsdk.Description("Subgenre fragment to match within track genres."),
			),
			mcpsdk.WithInteger("limit",
				mcpsdk.Description("Maximum records to return. Defaults to 50."),
			),
		),
		getFavoriteTracksHandler(db),
	)

	s.AddTool(
		mcpsdk.NewTool(getTopRatedAlbumsToolName,
			mcpsdk.WithDescription("Retrieve highest-rated albums ordered by personal rating, optionally filtered by genre."),
			mcpsdk.WithNumber("min_rating",
				mcpsdk.Description("Minimum user rating on a 5.0 scale. Defaults to 4.0."),
			),
			mcpsdk.WithString("genre",
				mcpsdk.Description("Subgenre fragment to match within album genres."),
			),
		),
		getTopRatedAlbumsHandler(db),
	)

	s.AddTool(
		mcpsdk.NewTool(getGenreDistributionToolName,
			mcpsdk.WithDescription("Summarize all subgenres present in the local music graph by frequency."),
		),
		getGenreDistributionHandler(db),
	)

	s.AddTool(
		mcpsdk.NewTool(getAlbumTracksToolName,
			mcpsdk.WithDescription("Retrieve the structured tracklist for a specific album."),
			mcpsdk.WithString("artist",
				mcpsdk.Required(),
				mcpsdk.Description("The album artist name."),
			),
			mcpsdk.WithString("album",
				mcpsdk.Required(),
				mcpsdk.Description("The album title."),
			),
		),
		getAlbumTracksHandler(db),
	)

	s.AddTool(
		mcpsdk.NewTool(getTasteAdjacenciesToolName,
			mcpsdk.WithDescription("Return local artist affinity and micro-genre topography context for discovery. "+recommendationToolInstructions),
			mcpsdk.WithArray("seed_artists",
				mcpsdk.Description("Optional artist names to build outward from. If omitted or empty, the user's top high-affinity artists are used."),
				mcpsdk.WithStringItems(),
			),
			mcpsdk.WithString("target_vibe",
				mcpsdk.Description("Optional sonic, technical, or mood descriptor to guide the discovery search."),
			),
		),
		getTasteAdjacenciesHandler(db),
	)

	s.AddTool(
		mcpsdk.NewTool(getVerifiedCandidatesToolName,
			mcpsdk.WithDescription("Search live MusicBrainz recording metadata by a raw canonical vibe or semantic fallback tags, then exclude artists, albums, and tracks already present in the local library. For abstract, non-canonical phrases, translate the phrase into canonical MusicBrainz genre tags before calling this tool; for example, map \"erratic rhythm section\" to fallback_tags [\"math rock\", \"idm\", \"breakcore\"]. Provide target_vibe or fallback_tags. "+recommendationToolInstructions),
			mcpsdk.WithString("target_vibe",
				mcpsdk.Description("Optional raw vibe or canonical MusicBrainz genre tag. A comma-separated canonical tag list is also accepted. Use fallback_tags instead when the original phrase is abstract or unlikely to be a MusicBrainz tag."),
			),
			mcpsdk.WithArray("fallback_tags",
				mcpsdk.Description("Optional semantic fallback as canonical MusicBrainz genre tags. When provided, these tags take precedence over target_vibe and are queried together. Example: [\"math rock\", \"idm\", \"breakcore\"]."),
				mcpsdk.MinItems(1),
				mcpsdk.UniqueItems(true),
				mcpsdk.WithStringItems(mcpsdk.MinLength(1)),
			),
			mcpsdk.WithInteger("limit",
				mcpsdk.Description("Maximum candidates to return. Defaults to 5 and is capped at 50."),
			),
		),
		getVerifiedDiscoveryCandidatesHandler(db, discovery),
	)

	s.AddTool(
		mcpsdk.NewTool(logAlbumRatingToolName,
			mcpsdk.WithDescription("Persist a local album rating directly to albums.user_rating using Specification 07 clean key normalization; insert a UUID-backed album row if absent. "+recommendationToolInstructions),
			mcpsdk.WithString("artist",
				mcpsdk.Required(),
				mcpsdk.Description("The album artist name. The server normalizes by lowercasing, converting & to and, and stripping punctuation."),
			),
			mcpsdk.WithString("album",
				mcpsdk.Required(),
				mcpsdk.Description("The album title. The server normalizes by lowercasing, converting & to and, and stripping punctuation."),
			),
			mcpsdk.WithNumber("rating",
				mcpsdk.Required(),
				mcpsdk.Min(0.0),
				mcpsdk.Max(5.0),
				mcpsdk.Description("Personal critical score on the local 0.0 to 5.0 scale."),
			),
		),
		logAlbumRatingHandler(db),
	)
}

func getLibraryTracksHandler(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		query, validationErr := requireStringArgument(request, "query")
		if validationErr != nil {
			return validationErr, nil
		}

		query = strings.TrimSpace(query)
		if query == "" {
			return mcpsdk.NewToolResultText("Please provide a non-empty search query."), nil
		}

		matches, err := searchLibraryTracks(ctx, db, query)
		if err != nil {
			log.Printf("failed to search library tracks: %v", err)
			return mcpsdk.NewToolResultText("Unable to search library tracks right now."), nil
		}

		if len(matches) == 0 {
			return mcpsdk.NewToolResultText(fmt.Sprintf("No matching tracks found for %q.", query)), nil
		}

		var builder strings.Builder
		for _, match := range matches {
			fmt.Fprintf(
				&builder,
				"- **%s** by *%s* (Album: %s; Genres: %s)\n",
				match.title,
				match.artist,
				match.album,
				match.genresJSON,
			)
		}

		return mcpsdk.NewToolResultText(strings.TrimRight(builder.String(), "\n")), nil
	}
}

func getFavoriteTracksHandler(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		genre, validationErr := optionalStringArgument(request, "genre")
		if validationErr != nil {
			return validationErr, nil
		}
		limit, validationErr := optionalIntegerArgument(request, "limit", defaultFavoriteTrackLimit)
		if validationErr != nil {
			return validationErr, nil
		}
		if limit <= 0 {
			return mcpsdk.NewToolResultError("The limit argument must be a positive integer."), nil
		}

		genre = strings.TrimSpace(genre)
		tracks, err := fetchFavoriteTracks(ctx, db, genre, limit)
		if err != nil {
			log.Printf("failed to query favorite tracks: %v", err)
			return mcpsdk.NewToolResultError("Unable to query favorite tracks right now."), nil
		}

		if len(tracks) == 0 {
			if genre != "" {
				return mcpsdk.NewToolResultText(fmt.Sprintf("No favorite tracks found for genre %q.", genre)), nil
			}
			return mcpsdk.NewToolResultText("No favorite tracks found."), nil
		}

		return mcpsdk.NewToolResultText(formatFavoriteTracksMarkdown(tracks)), nil
	}
}

func getTopRatedAlbumsHandler(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		minRating, validationErr := optionalFloatArgument(request, "min_rating", defaultMinRating)
		if validationErr != nil {
			return validationErr, nil
		}
		genre, validationErr := optionalStringArgument(request, "genre")
		if validationErr != nil {
			return validationErr, nil
		}
		genre = strings.TrimSpace(genre)

		albums, err := fetchTopRatedAlbums(ctx, db, minRating, genre)
		if err != nil {
			log.Printf("failed to query top rated albums: %v", err)
			return mcpsdk.NewToolResultError("Unable to query top rated albums right now."), nil
		}

		if len(albums) == 0 {
			if genre != "" {
				return mcpsdk.NewToolResultText(fmt.Sprintf("No top rated albums found for genre %q.", genre)), nil
			}
			return mcpsdk.NewToolResultText("No top rated albums found."), nil
		}

		return mcpsdk.NewToolResultText(formatTopRatedAlbumsMarkdown(albums)), nil
	}
}

func getGenreDistributionHandler(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		if validationErr := validateArgumentObject(request); validationErr != nil {
			return validationErr, nil
		}

		distribution, err := fetchGenreDistribution(ctx, db)
		if err != nil {
			log.Printf("failed to query genre distribution: %v", err)
			return mcpsdk.NewToolResultError("Unable to query genre distribution right now."), nil
		}

		if len(distribution) == 0 {
			return mcpsdk.NewToolResultText("No genres found."), nil
		}

		return mcpsdk.NewToolResultText(formatGenreDistributionMarkdown(distribution)), nil
	}
}

func getAlbumTracksHandler(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		artist, validationErr := requireStringArgument(request, "artist")
		if validationErr != nil {
			return validationErr, nil
		}
		album, validationErr := requireStringArgument(request, "album")
		if validationErr != nil {
			return validationErr, nil
		}

		artist = strings.TrimSpace(artist)
		album = strings.TrimSpace(album)
		if artist == "" || album == "" {
			return mcpsdk.NewToolResultText("Please provide non-empty artist and album arguments."), nil
		}

		tracks, err := fetchAlbumTracks(ctx, db, artist, album)
		if err != nil {
			log.Printf("failed to query album tracks: %v", err)
			return mcpsdk.NewToolResultError("Unable to query album tracks right now."), nil
		}

		if len(tracks) == 0 {
			return mcpsdk.NewToolResultText(fmt.Sprintf("No tracks found for %q by %q.", album, artist)), nil
		}

		return mcpsdk.NewToolResultText(formatAlbumTracksMarkdown(tracks)), nil
	}
}

func getTasteAdjacenciesHandler(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		seedArtists, validationErr := optionalStringSliceArgument(request, "seed_artists")
		if validationErr != nil {
			return validationErr, nil
		}
		targetVibe, validationErr := optionalStringArgument(request, "target_vibe")
		if validationErr != nil {
			return validationErr, nil
		}

		profile, err := fetchTasteAdjacencyProfile(ctx, db, seedArtists)
		if err != nil {
			log.Printf("failed to query taste adjacency profile: %v", err)
			return mcpsdk.NewToolResultError("Unable to query taste adjacency context right now."), nil
		}

		return mcpsdk.NewToolResultText(formatTasteAdjacencyProfileMarkdown(profile, strings.TrimSpace(targetVibe))), nil
	}
}

func getVerifiedDiscoveryCandidatesHandler(db *sql.DB, discovery discoverySource) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		targetVibe, validationErr := optionalStringArgument(request, "target_vibe")
		if validationErr != nil {
			return validationErr, nil
		}
		fallbackTags, validationErr := optionalStringSliceArgument(request, "fallback_tags")
		if validationErr != nil {
			return validationErr, nil
		}
		limit, validationErr := optionalIntegerArgument(request, "limit", defaultDiscoveryCandidateLimit)
		if validationErr != nil {
			return validationErr, nil
		}

		fallbackTags = compactStrings(fallbackTags)
		searchTags := fallbackTags
		if len(searchTags) == 0 && strings.TrimSpace(targetVibe) != "" {
			searchTags = []string{targetVibe}
		}
		if len(searchTags) == 0 {
			return mcpsdk.NewToolResultError("Please provide a non-empty target_vibe or fallback_tags argument."), nil
		}
		if limit <= 0 {
			return mcpsdk.NewToolResultError("The limit argument must be a positive integer."), nil
		}
		limit = clampDiscoveryCandidateLimit(limit)

		exclusions, err := (&database.DB{Ctx: db}).GetExclusionListContext(ctx)
		if err != nil {
			log.Printf("failed to build discovery exclusion list: %v", err)
			return mcpsdk.NewToolResultError("Unable to build the discovery exclusion list right now."), nil
		}

		candidates, err := getVerifiedDiscoveryCandidates(ctx, discovery, searchTags, limit, exclusions)
		if err != nil {
			log.Printf("failed to get verified discovery candidates: %v", err)
			return mcpsdk.NewToolResultError("Unable to retrieve verified discovery candidates right now."), nil
		}

		payload, err := json.Marshal(verifiedDiscoveryCandidatesToolResult{
			Instructions:   recommendationToolInstructions,
			EffectiveLimit: limit,
			Candidates:     candidates,
		})
		if err != nil {
			log.Printf("failed to encode verified discovery candidates: %v", err)
			return mcpsdk.NewToolResultError("Unable to encode verified discovery candidates right now."), nil
		}

		return mcpsdk.NewToolResultText(string(payload)), nil
	}
}

// Deprecated: check_album_history is no longer registered as an MCP endpoint.
// Discovery callers must use get_verified_discovery_candidates so metadata and
// history exclusion are enforced before candidates enter the LLM context.
func checkAlbumHistoryHandler(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		artist, validationErr := requireStringArgument(request, "artist")
		if validationErr != nil {
			return validationErr, nil
		}
		album, validationErr := requireStringArgument(request, "album")
		if validationErr != nil {
			return validationErr, nil
		}

		artist = strings.TrimSpace(artist)
		album = strings.TrimSpace(album)
		if artist == "" || album == "" {
			return mcpsdk.NewToolResultText("Please provide non-empty artist and album arguments."), nil
		}

		history, err := checkAlbumHistory(ctx, db, artist, album)
		if err != nil {
			log.Printf("failed to check album history: %v", err)
			return mcpsdk.NewToolResultError("Unable to check album history right now."), nil
		}
		if history.cleanArtist == "" || history.cleanTitle == "" {
			return mcpsdk.NewToolResultText("Please provide artist and album arguments that normalize to non-empty lookup tokens."), nil
		}

		return mcpsdk.NewToolResultText(formatAlbumHistoryMarkdown(history)), nil
	}
}

func logAlbumRatingHandler(db *sql.DB) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		artist, validationErr := requireStringArgument(request, "artist")
		if validationErr != nil {
			return validationErr, nil
		}
		album, validationErr := requireStringArgument(request, "album")
		if validationErr != nil {
			return validationErr, nil
		}
		rating, validationErr := requireFloatArgument(request, "rating")
		if validationErr != nil {
			return validationErr, nil
		}

		artist = strings.TrimSpace(artist)
		album = strings.TrimSpace(album)
		if artist == "" || album == "" {
			return mcpsdk.NewToolResultError("Please provide non-empty artist and album arguments."), nil
		}
		if rating < 0 || rating > 5 {
			return mcpsdk.NewToolResultError("The rating argument must be between 0 and 5."), nil
		}

		result, err := logAlbumRating(ctx, db, artist, album, rating)
		if err != nil {
			log.Printf("failed to log album rating: %v", err)
			return mcpsdk.NewToolResultError("Unable to log album rating right now."), nil
		}
		if result.cleanArtist == "" || result.cleanTitle == "" {
			return mcpsdk.NewToolResultError("Please provide artist and album arguments that normalize to non-empty lookup tokens."), nil
		}

		return mcpsdk.NewToolResultText(formatAlbumRatingLogMarkdown(result)), nil
	}
}

type libraryTrack struct {
	title      string
	artist     string
	album      string
	genresJSON string
}

type favoriteTrack struct {
	title      string
	artist     string
	album      string
	genresJSON string
}

type topRatedAlbum struct {
	title      string
	artist     string
	genresJSON string
	userRating float64
}

type genreCount struct {
	subgenre string
	total    int
}

type albumTrack struct {
	title      string
	artist     string
	isFavorite bool
}

type tasteAdjacencyProfile struct {
	seedArtists        []string
	missingSeedArtists []string
	artistAffinities   []artistAffinityContext
	genreTopography    []genreTopographyContext
}

type artistAffinityContext struct {
	artist              string
	cleanArtist         string
	favoriteTracksCount int
	dislikedTracksCount int
	avgUserRating       sql.NullFloat64
	curvedAffinityScore float64
}

type genreTopographyContext struct {
	subgenre            string
	totalTracks         int
	favoriteTracksCount int
	dislikedTracksCount int
	avgAlbumRating      sql.NullFloat64
}

type albumHistory struct {
	requestedArtist     string
	requestedAlbum      string
	cleanArtist         string
	cleanTitle          string
	albumFound          bool
	albumID             string
	albumTitle          string
	albumArtist         string
	userRating          sql.NullFloat64
	artistTrackCount    int
	artistFavoriteCount int
	artistRatedAlbums   int
	artistAvgRating     sql.NullFloat64
}

type albumRatingLog struct {
	albumID     string
	title       string
	artist      string
	cleanTitle  string
	cleanArtist string
	rating      float64
	inserted    bool
	rowsUpdated int64
}

type verifiedDiscoveryCandidatesToolResult struct {
	Instructions   string               `json:"instructions"`
	EffectiveLimit int                  `json:"effective_limit"`
	Candidates     []DiscoveryCandidate `json:"candidates"`
}

func searchLibraryTracks(ctx context.Context, db *sql.DB, query string) ([]libraryTrack, error) {
	const stmt = `
		SELECT title, artist, album, genres
		FROM tracks
		WHERE clean_title LIKE ? ESCAPE '\' OR clean_artist LIKE ? ESCAPE '\'
		ORDER BY artist COLLATE NOCASE, title COLLATE NOCASE
		LIMIT ?`

	cleanQuery, err := utils.NormalizeSearchText(query)
	if err != nil {
		return nil, err
	}
	if cleanQuery == "" {
		return nil, nil
	}

	likeQuery := "%" + escapeLikePattern(cleanQuery) + "%"
	rows, err := db.QueryContext(ctx, stmt, likeQuery, likeQuery, libraryTrackCap)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []libraryTrack
	for rows.Next() {
		var track libraryTrack
		var album sql.NullString
		var genres sql.NullString

		if err := rows.Scan(&track.title, &track.artist, &album, &genres); err != nil {
			return nil, err
		}

		track.album = "Unknown"
		if album.Valid && strings.TrimSpace(album.String) != "" {
			track.album = album.String
		}
		track.genresJSON = formatGenresJSON(genres)

		tracks = append(tracks, track)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tracks, nil
}

func fetchFavoriteTracks(ctx context.Context, db *sql.DB, genre string, limit int) ([]favoriteTrack, error) {
	const baseStmt = `
		SELECT title, artist, album, genres
		FROM tracks
		WHERE COALESCE(is_favorite, 0) = 1`
	const genreFilter = `
			AND EXISTS (
				SELECT 1
				FROM json_each(CASE WHEN json_valid(tracks.genres) THEN tracks.genres ELSE '[]' END) AS track_genre
				WHERE lower(CAST(track_genre.value AS TEXT)) LIKE ? ESCAPE '\'
			)`
	const orderLimit = `
		ORDER BY artist COLLATE NOCASE, title COLLATE NOCASE
		LIMIT ?`

	args := []any{}
	stmt := baseStmt
	genre = strings.ToLower(strings.TrimSpace(genre))
	if genre != "" {
		stmt += genreFilter
		args = append(args, "%"+escapeLikePattern(genre)+"%")
	}
	stmt += orderLimit
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []favoriteTrack
	for rows.Next() {
		var track favoriteTrack
		var album sql.NullString
		var genres sql.NullString

		if err := rows.Scan(&track.title, &track.artist, &album, &genres); err != nil {
			return nil, err
		}

		track.album = "Unknown"
		if album.Valid && strings.TrimSpace(album.String) != "" {
			track.album = album.String
		}
		track.genresJSON = formatGenresJSON(genres)

		tracks = append(tracks, track)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tracks, nil
}

func fetchTopRatedAlbums(ctx context.Context, db *sql.DB, minRating float64, genre string) ([]topRatedAlbum, error) {
	const baseStmt = `
		SELECT title, artist, genres, user_rating
		FROM albums
		WHERE user_rating IS NOT NULL
			AND user_rating >= ?`
	const genreFilter = `
			AND EXISTS (
				SELECT 1
				FROM json_each(CASE WHEN json_valid(albums.genres) THEN albums.genres ELSE '[]' END) AS album_genre
				WHERE lower(CAST(album_genre.value AS TEXT)) LIKE ? ESCAPE '\'
			)`
	const orderLimit = `
		ORDER BY user_rating DESC, artist COLLATE NOCASE, title COLLATE NOCASE
		LIMIT ?`

	args := []any{minRating}
	stmt := baseStmt
	genre = strings.ToLower(strings.TrimSpace(genre))
	if genre != "" {
		stmt += genreFilter
		args = append(args, "%"+escapeLikePattern(genre)+"%")
	}
	stmt += orderLimit
	args = append(args, defaultMaxAlbums)

	rows, err := db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var albums []topRatedAlbum
	for rows.Next() {
		var album topRatedAlbum
		var genres sql.NullString

		if err := rows.Scan(&album.title, &album.artist, &genres, &album.userRating); err != nil {
			return nil, err
		}

		album.genresJSON = formatGenresJSON(genres)
		albums = append(albums, album)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return albums, nil
}

func fetchGenreDistribution(ctx context.Context, db *sql.DB) ([]genreCount, error) {
	const stmt = `
		SELECT genres FROM albums WHERE genres IS NOT NULL AND trim(genres) != ''
		UNION ALL
		SELECT genres FROM tracks WHERE genres IS NOT NULL AND trim(genres) != ''`

	rows, err := db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	frequencies := make(map[string]int)
	for rows.Next() {
		var rawGenres string
		if err := rows.Scan(&rawGenres); err != nil {
			return nil, err
		}

		for _, genre := range parseGenreList(rawGenres) {
			genre = strings.ToLower(strings.TrimSpace(genre))
			if genre == "" {
				continue
			}
			frequencies[genre]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	distribution := make([]genreCount, 0, len(frequencies))
	for subgenre, total := range frequencies {
		distribution = append(distribution, genreCount{subgenre: subgenre, total: total})
	}
	sort.Slice(distribution, func(i, j int) bool {
		if distribution[i].total == distribution[j].total {
			return distribution[i].subgenre < distribution[j].subgenre
		}
		return distribution[i].total > distribution[j].total
	})

	return distribution, nil
}

func fetchAlbumTracks(ctx context.Context, db *sql.DB, artist, album string) ([]albumTrack, error) {
	const stmt = `
		WITH matched_albums AS (
			SELECT id, title, clean_artist
			FROM albums
			WHERE clean_title = ? AND clean_artist = ?
		)
		SELECT track.title, track.artist, COALESCE(track.is_favorite, 0)
		FROM tracks AS track
		WHERE track.album_id IN (SELECT id FROM matched_albums)
			OR EXISTS (
				SELECT 1
				FROM matched_albums AS album
				WHERE track.album = album.title
					AND track.clean_artist = album.clean_artist
			)
		ORDER BY track.rowid`

	cleanAlbum, err := utils.NormalizeSearchText(album)
	if err != nil {
		return nil, err
	}
	cleanArtist, err := utils.NormalizeSearchText(artist)
	if err != nil {
		return nil, err
	}
	if cleanAlbum == "" || cleanArtist == "" {
		return nil, nil
	}

	rows, err := db.QueryContext(ctx, stmt, cleanAlbum, cleanArtist)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []albumTrack
	for rows.Next() {
		var track albumTrack
		var isFavorite int

		if err := rows.Scan(&track.title, &track.artist, &isFavorite); err != nil {
			return nil, err
		}

		track.isFavorite = isFavorite == 1
		tracks = append(tracks, track)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tracks, nil
}

func fetchTasteAdjacencyProfile(ctx context.Context, db *sql.DB, seedArtists []string) (tasteAdjacencyProfile, error) {
	seedArtists = compactStrings(seedArtists)
	profile := tasteAdjacencyProfile{
		seedArtists: seedArtists,
	}

	var err error
	if len(seedArtists) == 0 {
		profile.artistAffinities, err = fetchTopArtistAffinityContexts(ctx, db, defaultTasteArtistLimit)
		if err != nil {
			return tasteAdjacencyProfile{}, err
		}
	} else {
		seenSeeds := make(map[string]bool, len(seedArtists))
		for _, seedArtist := range seedArtists {
			cleanArtist, err := utils.NormalizeSearchText(seedArtist)
			if err != nil {
				return tasteAdjacencyProfile{}, err
			}
			if cleanArtist == "" || seenSeeds[cleanArtist] {
				continue
			}
			seenSeeds[cleanArtist] = true

			affinity, found, err := fetchArtistAffinityContext(ctx, db, cleanArtist)
			if err != nil {
				return tasteAdjacencyProfile{}, err
			}
			if !found {
				profile.missingSeedArtists = append(profile.missingSeedArtists, seedArtist)
				continue
			}
			profile.artistAffinities = append(profile.artistAffinities, affinity)
		}
	}

	profile.genreTopography, err = fetchTopGenreTopographyContexts(ctx, db, defaultTasteGenreLimit)
	if err != nil {
		return tasteAdjacencyProfile{}, err
	}

	return profile, nil
}

func fetchTopArtistAffinityContexts(ctx context.Context, db *sql.DB, limit int) ([]artistAffinityContext, error) {
	const stmt = `
		SELECT artist, clean_artist, favorite_tracks_count, disliked_tracks_count, avg_user_rating, curved_affinity_score
		FROM v_artist_affinity
		ORDER BY curved_affinity_score DESC, favorite_tracks_count DESC, artist COLLATE NOCASE ASC
		LIMIT ?`

	rows, err := db.QueryContext(ctx, stmt, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanArtistAffinityContexts(rows)
}

func fetchArtistAffinityContext(ctx context.Context, db *sql.DB, cleanArtist string) (artistAffinityContext, bool, error) {
	const stmt = `
		SELECT artist, clean_artist, favorite_tracks_count, disliked_tracks_count, avg_user_rating, curved_affinity_score
		FROM v_artist_affinity
		WHERE clean_artist = ?
		ORDER BY curved_affinity_score DESC, favorite_tracks_count DESC, artist COLLATE NOCASE ASC
		LIMIT 1`

	rows, err := db.QueryContext(ctx, stmt, cleanArtist)
	if err != nil {
		return artistAffinityContext{}, false, err
	}
	defer rows.Close()

	affinities, err := scanArtistAffinityContexts(rows)
	if err != nil {
		return artistAffinityContext{}, false, err
	}
	if len(affinities) == 0 {
		return artistAffinityContext{}, false, nil
	}

	return affinities[0], true, nil
}

func scanArtistAffinityContexts(rows *sql.Rows) ([]artistAffinityContext, error) {
	var affinities []artistAffinityContext
	for rows.Next() {
		var affinity artistAffinityContext
		if err := rows.Scan(
			&affinity.artist,
			&affinity.cleanArtist,
			&affinity.favoriteTracksCount,
			&affinity.dislikedTracksCount,
			&affinity.avgUserRating,
			&affinity.curvedAffinityScore,
		); err != nil {
			return nil, err
		}
		affinities = append(affinities, affinity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return affinities, nil
}

func fetchTopGenreTopographyContexts(ctx context.Context, db *sql.DB, limit int) ([]genreTopographyContext, error) {
	const stmt = `
		SELECT subgenre, total_tracks, favorite_tracks_count, disliked_tracks_count, avg_album_rating
		FROM v_genre_topography
		ORDER BY favorite_tracks_count DESC, total_tracks DESC, subgenre COLLATE NOCASE ASC
		LIMIT ?`

	rows, err := db.QueryContext(ctx, stmt, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topography []genreTopographyContext
	for rows.Next() {
		var genre genreTopographyContext
		if err := rows.Scan(
			&genre.subgenre,
			&genre.totalTracks,
			&genre.favoriteTracksCount,
			&genre.dislikedTracksCount,
			&genre.avgAlbumRating,
		); err != nil {
			return nil, err
		}
		topography = append(topography, genre)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return topography, nil
}

func checkAlbumHistory(ctx context.Context, db *sql.DB, artist, album string) (albumHistory, error) {
	cleanArtist, cleanTitle, err := normalizeAlbumLookup(artist, album)
	if err != nil {
		return albumHistory{}, err
	}

	history := albumHistory{
		requestedArtist: artist,
		requestedAlbum:  album,
		cleanArtist:     cleanArtist,
		cleanTitle:      cleanTitle,
	}
	if cleanArtist == "" || cleanTitle == "" {
		return history, nil
	}

	err = db.QueryRowContext(ctx, `
		SELECT id, title, artist, user_rating
		FROM albums
		WHERE clean_title = ? AND clean_artist = ?
		ORDER BY CASE WHEN user_rating IS NOT NULL THEN 0 ELSE 1 END, rowid
		LIMIT 1`,
		cleanTitle,
		cleanArtist,
	).Scan(&history.albumID, &history.albumTitle, &history.albumArtist, &history.userRating)
	if err == nil {
		history.albumFound = true
	} else if err != sql.ErrNoRows {
		return albumHistory{}, err
	}

	if err := db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN COALESCE(is_favorite, 0) = 1 THEN 1 ELSE 0 END), 0)
		FROM tracks
		WHERE clean_artist = ?`,
		cleanArtist,
	).Scan(&history.artistTrackCount, &history.artistFavoriteCount); err != nil {
		return albumHistory{}, err
	}

	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*), AVG(user_rating)
		FROM albums
		WHERE clean_artist = ? AND user_rating IS NOT NULL`,
		cleanArtist,
	).Scan(&history.artistRatedAlbums, &history.artistAvgRating); err != nil {
		return albumHistory{}, err
	}

	return history, nil
}

func logAlbumRating(ctx context.Context, db *sql.DB, artist, album string, rating float64) (albumRatingLog, error) {
	if _, ok := finiteFloat(rating); !ok || rating < 0 || rating > 5 {
		return albumRatingLog{}, fmt.Errorf("rating must be between 0 and 5")
	}

	cleanArtist, cleanTitle, err := normalizeAlbumLookup(artist, album)
	if err != nil {
		return albumRatingLog{}, err
	}

	result := albumRatingLog{
		title:       strings.TrimSpace(album),
		artist:      strings.TrimSpace(artist),
		cleanTitle:  cleanTitle,
		cleanArtist: cleanArtist,
		rating:      rating,
	}
	if cleanArtist == "" || cleanTitle == "" {
		return result, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return albumRatingLog{}, err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx, `
		SELECT id, title, artist
		FROM albums
		WHERE clean_title = ? AND clean_artist = ?
		ORDER BY CASE WHEN user_rating IS NOT NULL THEN 0 ELSE 1 END, rowid
		LIMIT 1`,
		cleanTitle,
		cleanArtist,
	).Scan(&result.albumID, &result.title, &result.artist)
	if err == nil {
		updateResult, err := tx.ExecContext(ctx, `
			UPDATE albums
			SET user_rating = ?
			WHERE clean_title = ? AND clean_artist = ?`,
			rating,
			cleanTitle,
			cleanArtist,
		)
		if err != nil {
			return albumRatingLog{}, err
		}
		result.rowsUpdated, err = updateResult.RowsAffected()
		if err != nil {
			return albumRatingLog{}, err
		}
		if result.rowsUpdated == 0 {
			return albumRatingLog{}, fmt.Errorf("rating update affected no rows for clean album key %q/%q", cleanArtist, cleanTitle)
		}
		if err := tx.Commit(); err != nil {
			return albumRatingLog{}, err
		}
		return result, nil
	}
	if err != sql.ErrNoRows {
		return albumRatingLog{}, err
	}

	result.albumID = uuid.NewString()
	result.inserted = true
	insertResult, err := tx.ExecContext(ctx, `
		INSERT INTO albums (id, title, artist, clean_title, clean_artist, user_rating)
		VALUES (?, ?, ?, ?, ?, ?)`,
		result.albumID,
		result.title,
		result.artist,
		result.cleanTitle,
		result.cleanArtist,
		result.rating,
	)
	if err != nil {
		return albumRatingLog{}, err
	}
	result.rowsUpdated, err = insertResult.RowsAffected()
	if err != nil {
		return albumRatingLog{}, err
	}
	if err := tx.Commit(); err != nil {
		return albumRatingLog{}, err
	}

	return result, nil
}

func normalizeAlbumLookup(artist, album string) (string, string, error) {
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

func formatGenresJSON(genres sql.NullString) string {
	if !genres.Valid || strings.TrimSpace(genres.String) == "" {
		return "[]"
	}

	raw := strings.TrimSpace(genres.String)
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return "[]"
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(raw)); err != nil {
		return "[]"
	}

	return compact.String()
}

func parseGenreList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}

	return values
}

func formatFavoriteTracksMarkdown(tracks []favoriteTrack) string {
	var builder strings.Builder
	for _, track := range tracks {
		fmt.Fprintf(
			&builder,
			"- **%s** by *%s* (Album: %s; Genres: %s)\n",
			track.title,
			track.artist,
			track.album,
			track.genresJSON,
		)
	}

	return strings.TrimRight(builder.String(), "\n")
}

func formatTopRatedAlbumsMarkdown(albums []topRatedAlbum) string {
	var builder strings.Builder
	for _, album := range albums {
		fmt.Fprintf(
			&builder,
			"- **%s** by *%s* - %s/5 (Genres: %s)\n",
			album.title,
			album.artist,
			formatRating(album.userRating),
			album.genresJSON,
		)
	}

	return strings.TrimRight(builder.String(), "\n")
}

func formatGenreDistributionMarkdown(distribution []genreCount) string {
	var builder strings.Builder
	builder.WriteString("| Subgenre | Total Occurrences |\n")
	builder.WriteString("| --- | ---: |\n")
	for _, count := range distribution {
		fmt.Fprintf(
			&builder,
			"| %s | %d |\n",
			formatMarkdownTableCell(count.subgenre),
			count.total,
		)
	}

	return strings.TrimRight(builder.String(), "\n")
}

func formatAlbumTracksMarkdown(tracks []albumTrack) string {
	var builder strings.Builder
	for _, track := range tracks {
		favorite := ""
		if track.isFavorite {
			favorite = " ❤️"
		}
		fmt.Fprintf(&builder, "- **%s** by *%s*%s\n", track.title, track.artist, favorite)
	}

	return strings.TrimRight(builder.String(), "\n")
}

func formatTasteAdjacencyProfileMarkdown(profile tasteAdjacencyProfile, targetVibe string) string {
	var builder strings.Builder
	builder.WriteString("## Taste Adjacency Context\n")
	if targetVibe != "" {
		fmt.Fprintf(&builder, "Target vibe: %s\n", targetVibe)
	}
	if len(profile.seedArtists) == 0 {
		builder.WriteString("Seed mode: top high-affinity artists\n")
	} else {
		fmt.Fprintf(&builder, "Seed artists: %s\n", strings.Join(profile.seedArtists, ", "))
	}
	builder.WriteString("\n")

	builder.WriteString("### LLM Recommendation Contract\n")
	builder.WriteString("- Prioritize the sourcing hierarchy in the tool definition before broad web or memory claims.\n")
	builder.WriteString("- Cross-reference musician pedigree, side projects, guest/session personnel, and production credits.\n")
	builder.WriteString("- Rank by sonic topology overlap: technical density, rhythm, arrangement, and execution over generic genre labels.\n")
	builder.WriteString("- Final recommendation output must use exactly these groups: Direct Adjacencies and Cross-Genre Wildcards.\n\n")

	builder.WriteString("### Artist Affinity Matrix\n")
	if len(profile.artistAffinities) == 0 {
		builder.WriteString("No artist affinity rows found.\n")
	} else {
		builder.WriteString("| Artist | Clean Artist | Favorite Tracks | Disliked Tracks | Avg Rating | Curved Affinity |\n")
		builder.WriteString("| --- | --- | ---: | ---: | ---: | ---: |\n")
		for _, affinity := range profile.artistAffinities {
			fmt.Fprintf(
				&builder,
				"| %s | `%s` | %d | %d | %s | %s |\n",
				formatMarkdownTableCell(affinity.artist),
				affinity.cleanArtist,
				affinity.favoriteTracksCount,
				affinity.dislikedTracksCount,
				formatNullableRating(affinity.avgUserRating),
				formatRating(affinity.curvedAffinityScore),
			)
		}
	}
	if len(profile.missingSeedArtists) > 0 {
		fmt.Fprintf(&builder, "\nMissing seed artists in local affinity view: %s\n", strings.Join(profile.missingSeedArtists, ", "))
	}
	builder.WriteString("\n")

	builder.WriteString("### Micro-Genre Topography\n")
	if len(profile.genreTopography) == 0 {
		builder.WriteString("No genre topography rows found.\n")
	} else {
		builder.WriteString("| Subgenre | Total Tracks | Favorite Tracks | Disliked Tracks | Avg Album Rating |\n")
		builder.WriteString("| --- | ---: | ---: | ---: | ---: |\n")
		for _, genre := range profile.genreTopography {
			fmt.Fprintf(
				&builder,
				"| %s | %d | %d | %d | %s |\n",
				formatMarkdownTableCell(genre.subgenre),
				genre.totalTracks,
				genre.favoriteTracksCount,
				genre.dislikedTracksCount,
				formatNullableRating(genre.avgAlbumRating),
			)
		}
	}

	return strings.TrimRight(builder.String(), "\n")
}

func formatAlbumHistoryMarkdown(history albumHistory) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Album history for **%s** by *%s*\n", history.requestedAlbum, history.requestedArtist)
	fmt.Fprintf(&builder, "- Clean lookup: `%s` / `%s`\n", history.cleanArtist, history.cleanTitle)
	if history.albumFound {
		fmt.Fprintf(&builder, "- Album row: found `%s` as **%s** by *%s*\n", history.albumID, history.albumTitle, history.albumArtist)
		fmt.Fprintf(&builder, "- User rating: %s\n", formatNullableRating(history.userRating))
	} else {
		builder.WriteString("- Album row: not found\n")
	}
	fmt.Fprintf(
		&builder,
		"- Artist track footprint: %d tracks; %d explicit favorites\n",
		history.artistTrackCount,
		history.artistFavoriteCount,
	)
	if history.artistRatedAlbums > 0 {
		fmt.Fprintf(
			&builder,
			"- Artist rating footprint: %d rated albums; average %s\n",
			history.artistRatedAlbums,
			formatNullableRating(history.artistAvgRating),
		)
	} else {
		builder.WriteString("- Artist rating footprint: no rated albums\n")
	}

	return strings.TrimRight(builder.String(), "\n")
}

func formatAlbumRatingLogMarkdown(result albumRatingLog) string {
	action := "Updated"
	if result.inserted {
		action = "Inserted"
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "%s **%s** by *%s* at %s/5.\n", action, result.title, result.artist, formatRating(result.rating))
	fmt.Fprintf(&builder, "- Album ID: `%s`\n", result.albumID)
	fmt.Fprintf(&builder, "- Clean lookup: `%s` / `%s`\n", result.cleanArtist, result.cleanTitle)
	if result.inserted {
		fmt.Fprintf(&builder, "- albums rows inserted: %d\n", result.rowsUpdated)
	} else {
		fmt.Fprintf(&builder, "- albums rows updated: %d\n", result.rowsUpdated)
	}

	return strings.TrimRight(builder.String(), "\n")
}

func formatRating(rating float64) string {
	formatted := strconv.FormatFloat(rating, 'f', 2, 64)
	formatted = strings.TrimRight(formatted, "0")
	return strings.TrimRight(formatted, ".")
}

func formatNullableRating(rating sql.NullFloat64) string {
	if !rating.Valid {
		return "not rated"
	}
	return formatRating(rating.Float64) + "/5"
}

func formatMarkdownTableCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func compactStrings(values []string) []string {
	compacted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		compacted = append(compacted, value)
	}
	return compacted
}

func clampDiscoveryCandidateLimit(limit int) int {
	if limit > maxDiscoveryCandidateLimit {
		return maxDiscoveryCandidateLimit
	}
	return limit
}

func escapeLikePattern(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch r {
		case '\\', '%', '_':
			builder.WriteRune('\\')
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func validateArgumentObject(request mcpsdk.CallToolRequest) *mcpsdk.CallToolResult {
	_, validationErr := requestArguments(request)
	return validationErr
}

func requestArguments(request mcpsdk.CallToolRequest) (map[string]any, *mcpsdk.CallToolResult) {
	raw := request.GetRawArguments()
	if raw == nil {
		return map[string]any{}, nil
	}

	if args, ok := raw.(map[string]any); ok {
		return args, nil
	}

	if rawJSON, ok := raw.(json.RawMessage); ok {
		var args map[string]any
		if err := json.Unmarshal(rawJSON, &args); err != nil {
			return nil, mcpsdk.NewToolResultError("Tool arguments must be a JSON object.")
		}
		if args == nil {
			return map[string]any{}, nil
		}
		return args, nil
	}

	return nil, mcpsdk.NewToolResultError("Tool arguments must be a JSON object.")
}

func requireStringArgument(request mcpsdk.CallToolRequest, key string) (string, *mcpsdk.CallToolResult) {
	args, validationErr := requestArguments(request)
	if validationErr != nil {
		return "", validationErr
	}

	value, ok := args[key]
	if !ok || value == nil {
		return "", mcpsdk.NewToolResultError(fmt.Sprintf("The %s argument is required.", key))
	}

	stringValue, ok := value.(string)
	if !ok {
		return "", mcpsdk.NewToolResultError(fmt.Sprintf("The %s argument must be a string.", key))
	}

	return stringValue, nil
}

func optionalStringArgument(request mcpsdk.CallToolRequest, key string) (string, *mcpsdk.CallToolResult) {
	args, validationErr := requestArguments(request)
	if validationErr != nil {
		return "", validationErr
	}

	value, ok := args[key]
	if !ok || value == nil {
		return "", nil
	}

	stringValue, ok := value.(string)
	if !ok {
		return "", mcpsdk.NewToolResultError(fmt.Sprintf("The %s argument must be a string.", key))
	}

	return stringValue, nil
}

func optionalStringSliceArgument(request mcpsdk.CallToolRequest, key string) ([]string, *mcpsdk.CallToolResult) {
	args, validationErr := requestArguments(request)
	if validationErr != nil {
		return nil, validationErr
	}

	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}

	switch typedValue := value.(type) {
	case []string:
		return typedValue, nil
	case []any:
		values := make([]string, 0, len(typedValue))
		for _, item := range typedValue {
			stringItem, ok := item.(string)
			if !ok {
				return nil, mcpsdk.NewToolResultError(fmt.Sprintf("The %s argument must be an array of strings.", key))
			}
			values = append(values, stringItem)
		}
		return values, nil
	default:
		return nil, mcpsdk.NewToolResultError(fmt.Sprintf("The %s argument must be an array of strings.", key))
	}
}

func optionalIntegerArgument(request mcpsdk.CallToolRequest, key string, defaultValue int) (int, *mcpsdk.CallToolResult) {
	args, validationErr := requestArguments(request)
	if validationErr != nil {
		return 0, validationErr
	}

	value, ok := args[key]
	if !ok || value == nil {
		return defaultValue, nil
	}

	intValue, ok := integerArgumentValue(value)
	if !ok {
		return 0, mcpsdk.NewToolResultError(fmt.Sprintf("The %s argument must be an integer.", key))
	}

	return intValue, nil
}

func requireFloatArgument(request mcpsdk.CallToolRequest, key string) (float64, *mcpsdk.CallToolResult) {
	args, validationErr := requestArguments(request)
	if validationErr != nil {
		return 0, validationErr
	}

	value, ok := args[key]
	if !ok || value == nil {
		return 0, mcpsdk.NewToolResultError(fmt.Sprintf("The %s argument is required.", key))
	}

	floatValue, ok := floatArgumentValue(value)
	if !ok {
		return 0, mcpsdk.NewToolResultError(fmt.Sprintf("The %s argument must be a number.", key))
	}

	return floatValue, nil
}

func optionalFloatArgument(request mcpsdk.CallToolRequest, key string, defaultValue float64) (float64, *mcpsdk.CallToolResult) {
	args, validationErr := requestArguments(request)
	if validationErr != nil {
		return 0, validationErr
	}

	value, ok := args[key]
	if !ok || value == nil {
		return defaultValue, nil
	}

	floatValue, ok := floatArgumentValue(value)
	if !ok {
		return 0, mcpsdk.NewToolResultError(fmt.Sprintf("The %s argument must be a number.", key))
	}

	return floatValue, nil
}

func integerArgumentValue(value any) (int, bool) {
	const maxInt = int(^uint(0) >> 1)
	const minInt = -maxInt - 1

	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		if v < int64(minInt) || v > int64(maxInt) {
			return 0, false
		}
		return int(v), true
	case uint:
		if uint64(v) > uint64(maxInt) {
			return 0, false
		}
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		if uint64(v) > uint64(maxInt) {
			return 0, false
		}
		return int(v), true
	case uint64:
		if v > uint64(maxInt) {
			return 0, false
		}
		return int(v), true
	case float32:
		return integerFromFloat(float64(v))
	case float64:
		return integerFromFloat(v)
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, false
		}
		if parsed < int64(minInt) || parsed > int64(maxInt) {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func integerFromFloat(value float64) (int, bool) {
	const maxInt = int(^uint(0) >> 1)
	const minInt = -maxInt - 1

	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	if value < float64(minInt) || value > float64(maxInt) {
		return 0, false
	}
	return int(value), true
}

func floatArgumentValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return finiteFloat(v)
	case float32:
		return finiteFloat(float64(v))
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return finiteFloat(parsed)
	default:
		return 0, false
	}
}

func finiteFloat(value float64) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}
