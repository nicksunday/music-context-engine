package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/nicksunday/music-context-platform/internal/database"
	"github.com/nicksunday/music-context-platform/internal/enrich"
	"github.com/nicksunday/music-context-platform/internal/ingest"
	mcpserver "github.com/nicksunday/music-context-platform/internal/mcp"
)

const (
	profileArtistLimit = 10
	profileGenreLimit  = 5
	databaseFlagUsage  = "SQLite database file path (overrides MUSIC_VAULT_DB_PATH; defaults to data/music_vault.db)"
)

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "ingest":
		runIngest(os.Args[2:])
	case "enrich":
		runEnrich(os.Args[2:])
	case "optimize":
		runOptimize(os.Args[2:])
	case "profile":
		runProfile(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func runIngest(args []string) {
	flags := flag.NewFlagSet("ingest", flag.ExitOnError)
	var dbPath string
	registerDatabaseFlag(flags, &dbPath)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: music-vault ingest [--db path] [csv-file ...]\n")
	}
	if err := flags.Parse(args); err != nil {
		log.Fatalf("failed to parse ingest args: %v", err)
	}

	filePaths := flags.Args()
	if len(filePaths) == 0 {
		flags.Usage()
		os.Exit(2)
	}

	db, err := openDatabase(dbPath)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Ctx.Close()

	for _, filePath := range filePaths {
		header, err := ingest.ReadCSVHeader(filePath)
		if err != nil {
			log.Fatalf("failed to read CSV header from %s: %v", filePath, err)
		}

		source, err := ingest.DetectSource(header)
		if err != nil {
			log.Fatalf("%s: %v", filePath, err)
		}

		fmt.Printf("Ingesting %s as %s\n", filePath, source)
		switch source {
		case ingest.SourceLastFM:
			result, err := ingest.IngestLastFMFavorites(db, filePath, log.Default())
			if err != nil {
				log.Fatalf("Last.fm ingestion failed for %s: %v", filePath, err)
			}
			fmt.Printf(
				"Last.fm rows: %d; matched tracks: %d; created tracks: %d; unmatched rows: %d\n",
				result.Rows,
				result.MatchedTracks,
				result.CreatedTracks,
				result.UnmatchedRows,
			)
		case ingest.SourceYTMUploads:
			if err := ingest.IngestYT_Uploads(db, filePath); err != nil {
				log.Fatalf("YouTube Music uploads ingestion failed for %s: %v", filePath, err)
			}
		case ingest.SourceRYM:
			if err := ingest.IngestRYMExport(db, filePath); err != nil {
				log.Fatalf("RateYourMusic ingestion failed for %s: %v", filePath, err)
			}
		case ingest.SourceAppleMusicLikes:
			result, err := ingest.IngestAppleMusicLikes(db, filePath)
			if err != nil {
				log.Fatalf("Apple Music ingestion failed for %s: %v", filePath, err)
			}
			fmt.Printf(
				"Apple Music rows: %d; matched tracks: %d; created tracks: %d; unmatched rows: %d\n",
				result.Rows,
				result.MatchedTracks,
				result.CreatedTracks,
				result.UnmatchedRows,
			)
		case ingest.SourceAppleMusicFavorites:
			result, err := ingest.IngestAppleMusicFavorites(db, filePath)
			if err != nil {
				log.Fatalf("Apple Music favorites ingestion failed for %s: %v", filePath, err)
			}
			fmt.Printf(
				"Apple Music favorites rows: %d; matched tracks: %d; created tracks: %d; likes: %d; dislikes: %d; unmatched rows: %d\n",
				result.Rows,
				result.MatchedTracks,
				result.CreatedTracks,
				result.LikedRows,
				result.DislikedRows,
				result.UnmatchedRows,
			)
		case ingest.SourceAppleMusicLibraryActivity:
			result, err := ingest.IngestAppleMusicLibraryActivity(db, filePath)
			if err != nil {
				log.Fatalf("Apple Music library activity ingestion failed for %s: %v", filePath, err)
			}
			fmt.Printf(
				"Apple Music library activity rows: %d; track rows: %d; matched tracks: %d; created tracks: %d; positives: %d; dislikes: %d; unmatched track rows: %d; albums updated: %d\n",
				result.Rows,
				result.TrackRows,
				result.MatchedTracks,
				result.CreatedTracks,
				result.PositiveRows,
				result.DislikedRows,
				result.UnmatchedTrackRows,
				result.AlbumsUpdated,
			)
		case ingest.SourceAppleMusicLibraryTracks:
			result, err := ingest.IngestAppleMusicLibraryTracks(db, filePath)
			if err != nil {
				log.Fatalf("Apple Music library tracks ingestion failed for %s: %v", filePath, err)
			}
			fmt.Printf(
				"Apple Music library rows: %d; matched tracks: %d; created tracks: %d; positives: %d; dislikes: %d; unmatched rows: %d; albums updated: %d\n",
				result.Rows,
				result.MatchedTracks,
				result.CreatedTracks,
				result.PositiveRows,
				result.DislikedRows,
				result.UnmatchedRows,
				result.AlbumsUpdated,
			)
		case ingest.SourceAppleMusicPlayActivity:
			result, err := ingest.IngestAppleMusicPlayActivity(db, filePath)
			if err != nil {
				log.Fatalf("Apple Music play activity ingestion failed for %s: %v", filePath, err)
			}
			fmt.Printf(
				"Apple Music play activity rows: %d; written rows: %d; ambient rows skipped: %d; unmatched rows: %d\n",
				result.Rows,
				result.WrittenRows,
				result.SkippedAmbientRows,
				result.UnmatchedRows,
			)
		case ingest.SourceAppleMusicTrackHistory:
			result, err := ingest.IngestAppleMusicTrackPlayHistory(db, filePath)
			if err != nil {
				log.Fatalf("Apple Music track play history ingestion failed for %s: %v", filePath, err)
			}
			fmt.Printf(
				"Apple Music track play history rows: %d; written rows: %d; ambient rows skipped: %d; unmatched rows: %d\n",
				result.Rows,
				result.WrittenRows,
				result.SkippedAmbientRows,
				result.UnmatchedRows,
			)
		default:
			log.Fatalf("%s: unsupported source %q", filePath, source)
		}
	}

	printCounts(db)
}

func runEnrich(args []string) {
	flags := flag.NewFlagSet("enrich", flag.ExitOnError)
	var dbPath string
	registerDatabaseFlag(flags, &dbPath)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: music-vault enrich [--db path]\n")
	}
	if err := flags.Parse(args); err != nil {
		log.Fatalf("failed to parse enrich args: %v", err)
	}
	if len(flags.Args()) > 0 {
		flags.Usage()
		os.Exit(2)
	}

	db, err := openDatabase(dbPath)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Ctx.Close()

	config := enrich.DefaultConfig()
	config.LastFMAPIKey = os.Getenv(enrich.LastFMAPIKeyEnv)
	config.Logger = log.Default()

	if config.LastFMAPIKey == "" {
		fmt.Printf("%s is not set; Last.fm tags will be skipped.\n", enrich.LastFMAPIKeyEnv)
	}

	result, err := enrich.NewWorker(db.Ctx, config).Run(context.Background())
	if err != nil {
		log.Fatalf("metadata enrichment failed: %v", err)
	}

	fmt.Printf("Scanned albums: %d\n", result.AlbumsScanned)
	fmt.Printf("Albums enriched: %d\n", result.AlbumsUpdated)
	fmt.Printf("Scanned artists: %d\n", result.ArtistsScanned)
	fmt.Printf("Artists enriched: %d\n", result.ArtistsUpdated)
	fmt.Printf("Records updated: %d\n", result.RecordsUpdated)
}

func runOptimize(args []string) {
	flags := flag.NewFlagSet("optimize", flag.ExitOnError)
	var dbPath string
	registerDatabaseFlag(flags, &dbPath)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: music-vault optimize [--db path]\n")
	}
	if err := flags.Parse(args); err != nil {
		log.Fatalf("failed to parse optimize args: %v", err)
	}
	if len(flags.Args()) > 0 {
		flags.Usage()
		os.Exit(2)
	}

	db, err := openDatabase(dbPath)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Ctx.Close()

	purgedRows, err := database.ReconcileDuplicateAlbums(db)
	if err != nil {
		log.Fatalf("database optimization failed: %v", err)
	}

	fmt.Printf("Duplicate album rows purged: %d\n", purgedRows)
}

func runProfile(args []string) {
	flags := flag.NewFlagSet("profile", flag.ExitOnError)
	var genre string
	var limit int
	var dbPath string
	registerDatabaseFlag(flags, &dbPath)
	flags.StringVar(&genre, "genre", "", "filter high-affinity artists by genre")
	flags.IntVar(&limit, "limit", profileArtistLimit, "maximum artist rows to return")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: music-vault profile [--db path] [--genre genre] [--limit n]\n")
	}
	if err := flags.Parse(args); err != nil {
		log.Fatalf("failed to parse profile args: %v", err)
	}
	if len(flags.Args()) > 0 {
		flags.Usage()
		os.Exit(2)
	}
	if limit <= 0 {
		log.Fatalf("profile --limit must be greater than 0")
	}

	db, err := openDatabase(dbPath)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Ctx.Close()

	ctx := context.Background()
	genre = strings.TrimSpace(genre)
	if genre != "" {
		artists, err := database.FetchTopArtistAffinitiesByGenre(ctx, db.Ctx, genre, limit)
		if err != nil {
			log.Fatalf("failed to query genre-filtered artist affinity profile: %v", err)
		}

		fmt.Print(formatGenreProfileMarkdown(artists, genre, limit))
		return
	}

	artists, err := database.FetchTopArtistAffinities(ctx, db.Ctx, limit)
	if err != nil {
		log.Fatalf("failed to query artist affinity profile: %v", err)
	}

	genres, err := database.FetchTopGenreTopography(ctx, db.Ctx, profileGenreLimit)
	if err != nil {
		log.Fatalf("failed to query genre topography profile: %v", err)
	}

	fmt.Print(formatProfileMarkdown(artists, genres, limit))
}

func runServe(args []string) {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	var dbPath string
	registerDatabaseFlag(flags, &dbPath)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: music-vault serve [--db path]\n")
	}
	if err := flags.Parse(args); err != nil {
		log.Fatalf("failed to parse serve args: %v", err)
	}
	if len(flags.Args()) > 0 {
		flags.Usage()
		os.Exit(2)
	}

	db, err := openDatabase(dbPath)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Ctx.Close()

	if err := mcpserver.ServeStdio(db.Ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func registerDatabaseFlag(flags *flag.FlagSet, target *string) {
	flags.StringVar(target, "db", "", databaseFlagUsage)
}

func openDatabase(dbPath string) (*database.DBClient, error) {
	if strings.TrimSpace(dbPath) != "" {
		return database.InitDBAtPath(dbPath)
	}
	return database.InitDB("")
}

func printCounts(db *database.DBClient) {
	var trackCount int
	if err := db.Ctx.QueryRow("SELECT COUNT(DISTINCT id) FROM tracks").Scan(&trackCount); err != nil {
		log.Fatalf("failed to count tracks: %v", err)
	}

	var albumCount int
	if err := db.Ctx.QueryRow("SELECT COUNT(DISTINCT id) FROM albums").Scan(&albumCount); err != nil {
		log.Fatalf("failed to count albums: %v", err)
	}

	fmt.Printf("Total unique tracks: %d\n", trackCount)
	fmt.Printf("Total unique albums: %d\n", albumCount)
}

func formatProfileMarkdown(artists []database.ArtistAffinity, genres []database.GenreTopography, artistLimit int) string {
	var builder strings.Builder
	builder.WriteString("# Music Vault Profile\n\n")

	fmt.Fprintf(&builder, "## Top %d High-Affinity Artists\n\n", artistLimit)
	if len(artists) == 0 {
		builder.WriteString("_No artist affinity data found._\n\n")
	} else {
		writeArtistAffinityTable(&builder, artists)
		builder.WriteString("\n")
	}

	builder.WriteString("## Top 5 Micro-Genres\n\n")
	if len(genres) == 0 {
		builder.WriteString("_No genre topography data found._\n")
	} else {
		builder.WriteString("| Rank | Subgenre | Total Tracks | Favorite Tracks | Disliked Tracks | Avg Album Rating |\n")
		builder.WriteString("| ---: | --- | ---: | ---: | ---: | ---: |\n")
		for i, genre := range genres {
			fmt.Fprintf(
				&builder,
				"| %d | %s | %d | %d | %d | %s |\n",
				i+1,
				formatMarkdownTableCell(genre.Subgenre),
				genre.TotalTracks,
				genre.FavoriteTracksCount,
				genre.DislikedTracksCount,
				formatNullableRating(genre.AvgAlbumRating),
			)
		}
	}

	return builder.String()
}

func formatGenreProfileMarkdown(artists []database.ArtistAffinity, genre string, limit int) string {
	var builder strings.Builder
	fmt.Fprintf(
		&builder,
		"### Top %d Affinity Artists for Genre: %q\n\n",
		limit,
		genre,
	)

	if len(artists) == 0 {
		fmt.Fprintf(&builder, "_No artist affinity data found for genre %q._\n", genre)
		return builder.String()
	}

	writeArtistAffinityTable(&builder, artists)
	return builder.String()
}

func writeArtistAffinityTable(builder *strings.Builder, artists []database.ArtistAffinity) {
	builder.WriteString("| Rank | Artist | Favorite Tracks | Disliked Tracks | Avg User Rating | Curved Affinity Score |\n")
	builder.WriteString("| ---: | --- | ---: | ---: | ---: | ---: |\n")
	for i, artist := range artists {
		fmt.Fprintf(
			builder,
			"| %d | %s | %d | %d | %s | %s |\n",
			i+1,
			formatMarkdownTableCell(artist.Artist),
			artist.FavoriteTracksCount,
			artist.DislikedTracksCount,
			formatNullableRating(artist.AvgUserRating),
			formatProfileFloat(artist.CurvedAffinityScore),
		)
	}
}

func formatNullableRating(value sql.NullFloat64) string {
	if !value.Valid {
		return "N/A"
	}
	return formatProfileFloat(value.Float64)
}

func formatProfileFloat(value float64) string {
	formatted := strconv.FormatFloat(value, 'f', 2, 64)
	formatted = strings.TrimRight(formatted, "0")
	return strings.TrimRight(formatted, ".")
}

func formatMarkdownTableCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: music-vault <ingest|enrich|optimize|profile|serve> [args]")
}
