package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nicksunday/music-context-platform/internal/database"
	"github.com/nicksunday/music-context-platform/internal/enrich"
	"github.com/nicksunday/music-context-platform/internal/ingest"
	mcpserver "github.com/nicksunday/music-context-platform/internal/mcp"
	webserver "github.com/nicksunday/music-context-platform/internal/web"
)

const (
	profileArtistLimit      = 10
	profileGenreLimit       = 5
	defaultWebDaemonLogPath = "data/music-vault-web.log"
	databaseFlagUsage       = "SQLite database file path (overrides MUSIC_VAULT_DB_PATH; defaults to data/music_vault.db)"
)

var (
	findListeningPIDsForPort = findListeningPIDsWithLsof
	signalProcessByPID       = signalProcessWithTerm
	tcpListenAvailable       = canListenTCP
	restartSleep             = time.Sleep
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
	case "web":
		runWeb(os.Args[2:])
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
	maxRetries := flags.Int("max-retries", enrich.DefaultMaxTransientRetries, "max retries for a transient (5xx/429) upstream error before skipping the record for this run")
	retryDelay := flags.Duration("retry-delay", enrich.DefaultTransientRetryDelay, "base backoff delay between retries, doubled each attempt (e.g. 2s, 5s)")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: music-vault enrich [--db path] [--max-retries n] [--retry-delay duration]\n")
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
	config.MaxTransientRetries = *maxRetries
	config.TransientRetryDelay = *retryDelay

	if config.LastFMAPIKey == "" {
		fmt.Printf("%s is not set; Last.fm tags will be skipped.\n", enrich.LastFMAPIKeyEnv)
	}

	result, err := enrich.NewWorker(db.Ctx, config).Run(context.Background())
	if err != nil {
		log.Fatalf("metadata enrichment failed: %v", err)
	}

	fmt.Printf("Scanned albums: %d\n", result.AlbumsScanned)
	fmt.Printf("Albums enriched: %d\n", result.AlbumsUpdated)
	if result.AlbumsSkippedTransient > 0 {
		fmt.Printf("Albums skipped this run due to transient upstream errors (will retry next run): %d\n", result.AlbumsSkippedTransient)
	}
	fmt.Printf("Scanned artists: %d\n", result.ArtistsScanned)
	fmt.Printf("Artists enriched: %d\n", result.ArtistsUpdated)
	if result.ArtistsSkippedTransient > 0 {
		fmt.Printf("Artists skipped this run due to transient upstream errors (will retry next run): %d\n", result.ArtistsSkippedTransient)
	}
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

func runWeb(args []string) {
	flags := flag.NewFlagSet("web", flag.ExitOnError)
	var dbPath string
	var addr string
	var ollamaURL string
	var model string
	var ollamaTimeout time.Duration
	var restart bool
	var daemon bool
	var daemonLogPath string
	registerDatabaseFlag(flags, &dbPath)
	flags.StringVar(&addr, "addr", "127.0.0.1:8787", "HTTP listen address")
	flags.StringVar(&ollamaURL, "ollama-url", defaultOllamaURL(), "Ollama base URL")
	flags.StringVar(&model, "model", defaultWebModel(), "Ollama model name")
	flags.DurationVar(&ollamaTimeout, "ollama-timeout", 3*time.Minute, "maximum time to wait for an Ollama recommendation response")
	flags.BoolVar(&restart, "restart", false, "stop any existing listener on --addr before starting")
	flags.BoolVar(&daemon, "daemon", false, "start the web server in the background and return immediately")
	flags.StringVar(&daemonLogPath, "daemon-log", defaultWebDaemonLogPath, "log file used when --daemon is set")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: music-vault web [--db path] [--addr host:port] [--restart] [--daemon] [--daemon-log path] [--ollama-url url] [--model name] [--ollama-timeout duration]\n")
	}
	if err := flags.Parse(args); err != nil {
		log.Fatalf("failed to parse web args: %v", err)
	}
	if len(flags.Args()) > 0 {
		flags.Usage()
		os.Exit(2)
	}
	if restart {
		if err := restartWebListener(addr, 15*time.Second); err != nil {
			log.Fatalf("failed to restart web server: %v", err)
		}
	}
	if daemon {
		available, err := tcpListenAvailable(addr)
		if err != nil {
			log.Fatalf("failed to check web listen address: %v", err)
		}
		if !available {
			log.Fatalf("web listen address %s is already in use; pass --restart to stop the existing listener first", addr)
		}

		childArgs := buildWebDaemonArgs(dbPath, addr, ollamaURL, model, ollamaTimeout, false)
		pid, err := startWebDaemon(childArgs, daemonLogPath)
		if err != nil {
			log.Fatalf("failed to start web daemon: %v", err)
		}
		log.Printf("music-vault web daemon started with pid %d at http://%s (log: %s)", pid, addr, daemonLogPath)
		return
	}

	db, err := openDatabase(dbPath)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Ctx.Close()

	handler := webserver.NewServer(db.Ctx, webserver.Options{
		Recommender: webserver.NewMCPGroundedOllamaRecommender(db.Ctx, ollamaURL, model, ollamaTimeout).
			WithSimilarArtists(mcpserver.NewDefaultSimilarArtistSource()),
		ReleaseRadar: webserver.NewMusicBrainzReleaseRadar(),
		LinkResolver: webserver.NewAppleMusicLinker(),
		SongLinkResolver: webserver.FallbackSongLinkResolver{
			AppleMusic: webserver.NewAppleMusicLinker(),
			YouTube:    webserver.YouTubeSongLinker{},
		},
		AppleMusicDeveloperToken: os.Getenv("APPLE_MUSIC_DEVELOPER_TOKEN"),
		Model:                    model,
		OllamaURL:                ollamaURL,
		Timeout:                  ollamaTimeout,
	})

	server := &http.Server{Addr: addr, Handler: handler}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("web server listen error: %v", err)
	}
	shutdownSignals, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	log.Printf("music-vault web listening on http://%s", addr)
	if err := serveHTTPServer(server, listener, shutdownSignals.Done()); err != nil {
		log.Fatalf("web server error: %v", err)
	}
}

func buildWebDaemonArgs(
	dbPath string,
	addr string,
	ollamaURL string,
	model string,
	ollamaTimeout time.Duration,
	restart bool,
) []string {
	args := []string{
		"web",
		"--addr", addr,
		"--ollama-url", ollamaURL,
		"--model", model,
		"--ollama-timeout", ollamaTimeout.String(),
	}
	if strings.TrimSpace(dbPath) != "" {
		args = append(args, "--db", dbPath)
	}
	if restart {
		args = append(args, "--restart")
	}
	return args
}

func startWebDaemon(args []string, logPath string) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("find current executable: %w", err)
	}
	logFile, err := openDaemonLog(logPath)
	if err != nil {
		return 0, err
	}
	defer logFile.Close()

	command := exec.Command(executable, args...)
	command.Stdout = logFile
	command.Stderr = logFile
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if workingDirectory, err := os.Getwd(); err == nil {
		command.Dir = workingDirectory
	}
	if err := command.Start(); err != nil {
		return 0, fmt.Errorf("start daemon process: %w", err)
	}

	pid := command.Process.Pid
	if err := command.Process.Release(); err != nil {
		return pid, fmt.Errorf("release daemon process: %w", err)
	}
	return pid, nil
}

func openDaemonLog(logPath string) (*os.File, error) {
	logPath = strings.TrimSpace(logPath)
	if logPath == "" {
		logPath = defaultWebDaemonLogPath
	}
	if directory := filepath.Dir(logPath); directory != "." {
		if err := os.MkdirAll(directory, 0755); err != nil {
			return nil, fmt.Errorf("create daemon log directory %q: %w", directory, err)
		}
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open daemon log %q: %w", logPath, err)
	}
	return logFile, nil
}

func restartWebListener(addr string, timeout time.Duration) error {
	port, err := tcpListenPort(addr)
	if err != nil {
		return fmt.Errorf("parse web listen address %q: %w", addr, err)
	}

	pids, err := findListeningPIDsForPort(port)
	if err != nil {
		return err
	}
	pids = uniquePIDs(pids)
	if len(pids) == 0 {
		return nil
	}

	signaled := make([]int, 0, len(pids))
	self := os.Getpid()
	for _, pid := range pids {
		if pid == self {
			continue
		}
		if err := signalProcessByPID(pid); err != nil {
			return fmt.Errorf("stop listener pid %d: %w", pid, err)
		}
		signaled = append(signaled, pid)
	}
	if len(signaled) == 0 {
		return nil
	}
	log.Printf("stopped existing listener(s) on %s: %s", addr, formatPIDs(signaled))

	deadline := time.Now().Add(timeout)
	for {
		available, err := tcpListenAvailable(addr)
		if err != nil {
			return fmt.Errorf("check web listen address %q: %w", addr, err)
		}
		if available {
			return nil
		}
		if timeout <= 0 || time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s to stop after signaling pid(s): %s", addr, formatPIDs(signaled))
		}
		restartSleep(100 * time.Millisecond)
	}
}

func tcpListenPort(addr string) (string, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(port) == "" {
		return "", fmt.Errorf("missing port")
	}
	return port, nil
}

func findListeningPIDsWithLsof(port string) ([]int, error) {
	output, err := exec.Command("lsof", "-nP", "-tiTCP:"+port, "-sTCP:LISTEN").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(output) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("find listener on tcp port %s with lsof: %w", port, err)
	}

	lines := strings.Fields(string(output))
	pids := make([]int, 0, len(lines))
	for _, line := range lines {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			return nil, fmt.Errorf("parse lsof pid %q: %w", line, err)
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

func signalProcessWithTerm(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGTERM)
}

func canListenTCP(addr string) (bool, error) {
	listener, err := net.Listen("tcp", addr)
	if err == nil {
		if closeErr := listener.Close(); closeErr != nil {
			return false, closeErr
		}
		return true, nil
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return false, nil
	}
	return false, err
}

func uniquePIDs(pids []int) []int {
	seen := make(map[int]bool, len(pids))
	unique := make([]int, 0, len(pids))
	for _, pid := range pids {
		if pid <= 0 || seen[pid] {
			continue
		}
		seen[pid] = true
		unique = append(unique, pid)
	}
	return unique
}

func formatPIDs(pids []int) string {
	values := make([]string, 0, len(pids))
	for _, pid := range pids {
		values = append(values, strconv.Itoa(pid))
	}
	return strings.Join(values, ", ")
}

func serveHTTPServer(server *http.Server, listener net.Listener, shutdown <-chan struct{}) error {
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Serve(listener)
	}()

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-shutdown:
		graceContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(graceContext); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
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

func defaultOllamaURL() string {
	if value := strings.TrimSpace(os.Getenv("OLLAMA_HOST")); value != "" {
		return value
	}
	return "http://127.0.0.1:11434"
}

func defaultWebModel() string {
	if value := strings.TrimSpace(os.Getenv("MUSIC_VAULT_WEB_MODEL")); value != "" {
		return value
	}
	return "qwen3:latest"
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: music-vault <ingest|enrich|optimize|profile|serve|web> [args]")
}
