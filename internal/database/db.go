package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nicksunday/music-context-platform/internal/utils"
)

const (
	MusicVaultDBPathEnv = "MUSIC_VAULT_DB_PATH"
	defaultDatabasePath = "data/music_vault.db"
)

type DBClient struct {
	Ctx *sql.DB
}

func InitDB(defaultPath string) (*DBClient, error) {
	dbPath, err := ResolveDatabasePath(defaultPath)
	if err != nil {
		return nil, err
	}

	return initResolvedDB(dbPath)
}

func InitDBAtPath(dbPath string) (*DBClient, error) {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return nil, fmt.Errorf("database path must not be empty")
	}

	dbPath, err := expandHomeDatabasePath(dbPath)
	if err != nil {
		return nil, err
	}

	return initResolvedDB(dbPath)
}

func initResolvedDB(dbPath string) (*DBClient, error) {
	if err := ensureDatabaseDirectory(dbPath); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Schema uses natural keys to keep ingestion idempotent
	schema := `
	CREATE TABLE IF NOT EXISTS albums (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		artist TEXT NOT NULL,
		clean_title TEXT NOT NULL,
		clean_artist TEXT NOT NULL,
		genres TEXT,
		user_rating REAL,
		release_date INTEGER,
		track_count INTEGER
	);

	CREATE TABLE IF NOT EXISTS tracks (
		id TEXT PRIMARY KEY,
		album_id TEXT REFERENCES albums(id),
		title TEXT NOT NULL,
		album TEXT,
		artist TEXT NOT NULL,
		clean_title TEXT NOT NULL,
		clean_artist TEXT NOT NULL,
		genres TEXT,
		is_favorite INTEGER DEFAULT 0,
		is_disliked INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS apple_music_play_activity (
		id TEXT PRIMARY KEY,
		event_timestamp TEXT,
		track_description TEXT,
		song_name TEXT,
		artist TEXT,
		album TEXT,
		container_name TEXT,
		play_duration_ms INTEGER,
		end_reason_type TEXT,
		was_skipped INTEGER DEFAULT 0,
		source_type TEXT,
		is_user_initiated INTEGER
	);`

	_, err = db.Exec(schema)
	if err != nil {
		return nil, err
	}

	if err := dropAnalyticalViews(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureAlignedSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureSearchColumnsNormalized(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureIndexes(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureAnalyticalViews(db); err != nil {
		db.Close()
		return nil, err
	}

	return &DBClient{Ctx: db}, nil
}

func ResolveDatabasePath(defaultPath string) (string, error) {
	if envPath, ok := os.LookupEnv(MusicVaultDBPathEnv); ok {
		envPath = strings.TrimSpace(envPath)
		if envPath == "" {
			return resolveDefaultDatabasePath(defaultPath), nil
		}
		var err error
		envPath, err = expandHomeDatabasePath(envPath)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(envPath) {
			return "", fmt.Errorf("%s must be an absolute path when set", MusicVaultDBPathEnv)
		}
		return envPath, nil
	}

	return resolveDefaultDatabasePath(defaultPath), nil
}

func resolveDefaultDatabasePath(defaultPath string) string {
	if defaultPath == "" {
		return defaultDatabasePath
	}
	return defaultPath
}

func expandHomeDatabasePath(dbPath string) (string, error) {
	if dbPath != "~" && !strings.HasPrefix(dbPath, "~/") {
		return dbPath, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory for database path %q: %w", dbPath, err)
	}
	if dbPath == "~" {
		return homeDir, nil
	}
	return filepath.Join(homeDir, strings.TrimPrefix(dbPath, "~/")), nil
}

func ensureDatabaseDirectory(dbPath string) error {
	if dbPath == "" || dbPath == ":memory:" {
		return nil
	}

	dir := filepath.Dir(dbPath)
	if dir == "." || dir == "" {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create database directory %q: %w", dir, err)
	}
	return nil
}

func ensureAlignedSchema(db *sql.DB) error {
	if err := ensureAlbumsSchema(db); err != nil {
		return err
	}
	if err := ensureTracksSchema(db); err != nil {
		return err
	}
	if err := ensureAppleMusicPlayActivitySchema(db); err != nil {
		return err
	}
	return nil
}

func ensureAlbumsSchema(db *sql.DB) error {
	columns, err := tableColumnNames(db, "albums")
	if err != nil {
		return err
	}
	if hasExactColumns(columns, []string{
		"id",
		"title",
		"artist",
		"clean_title",
		"clean_artist",
		"genres",
		"user_rating",
		"release_date",
		"track_count",
	}) {
		return nil
	}

	return rebuildAlbumsTable(db, columnNameSet(columns))
}

func ensureTracksSchema(db *sql.DB) error {
	columns, err := tableColumnNames(db, "tracks")
	if err != nil {
		return err
	}
	if hasExactColumns(columns, []string{
		"id",
		"album_id",
		"title",
		"album",
		"artist",
		"clean_title",
		"clean_artist",
		"genres",
		"is_favorite",
		"is_disliked",
	}) {
		return nil
	}

	return rebuildTracksTable(db, columnNameSet(columns))
}

func ensureAppleMusicPlayActivitySchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS apple_music_play_activity (
			id TEXT PRIMARY KEY,
			event_timestamp TEXT,
			track_description TEXT,
			song_name TEXT,
			artist TEXT,
			album TEXT,
			container_name TEXT,
			play_duration_ms INTEGER,
			end_reason_type TEXT,
			was_skipped INTEGER DEFAULT 0,
			source_type TEXT,
			is_user_initiated INTEGER
		);`)
	if err != nil {
		return err
	}

	columns, err := tableColumnNames(db, "apple_music_play_activity")
	if err != nil {
		return err
	}
	columnSet := columnNameSet(columns)
	for _, column := range []struct {
		name       string
		definition string
	}{
		{"event_timestamp", "TEXT"},
		{"track_description", "TEXT"},
		{"song_name", "TEXT"},
		{"artist", "TEXT"},
		{"album", "TEXT"},
		{"container_name", "TEXT"},
		{"play_duration_ms", "INTEGER"},
		{"end_reason_type", "TEXT"},
		{"was_skipped", "INTEGER DEFAULT 0"},
		{"source_type", "TEXT"},
		{"is_user_initiated", "INTEGER"},
	} {
		if columnSet[column.name] {
			continue
		}
		if _, err := db.Exec(fmt.Sprintf("ALTER TABLE apple_music_play_activity ADD COLUMN %s %s", column.name, column.definition)); err != nil {
			return fmt.Errorf("failed to add apple_music_play_activity.%s column: %w", column.name, err)
		}
	}

	return nil
}

func ensureSearchColumnsNormalized(db *sql.DB) error {
	for _, table := range []string{"albums", "tracks"} {
		if err := refreshCleanSearchColumns(db, table); err != nil {
			return err
		}
	}

	return nil
}

func ensureIndexes(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_tracks_is_favorite ON tracks(is_favorite);
		CREATE INDEX IF NOT EXISTS idx_tracks_is_disliked ON tracks(is_disliked);
		CREATE INDEX IF NOT EXISTS idx_tracks_clean_title_artist ON tracks(clean_title, clean_artist);
		CREATE INDEX IF NOT EXISTS idx_albums_clean_title_artist ON albums(clean_title, clean_artist);
		CREATE INDEX IF NOT EXISTS idx_apple_music_play_activity_event_timestamp ON apple_music_play_activity(event_timestamp);
	`)
	return err
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("failed to inspect %s schema: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)

		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}

	if err := rows.Err(); err != nil {
		return false, err
	}

	return false, nil
}

func tableColumnNames(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("failed to inspect %s schema: %w", table, err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)

		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return columns, nil
}

func hasExactColumns(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func columnNameSet(columns []string) map[string]bool {
	set := make(map[string]bool, len(columns))
	for _, column := range columns {
		set[column] = true
	}
	return set
}

func rebuildAlbumsTable(db *sql.DB, columns map[string]bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		CREATE TABLE albums_new (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			artist TEXT NOT NULL,
			clean_title TEXT NOT NULL,
			clean_artist TEXT NOT NULL,
			genres TEXT,
			user_rating REAL,
			release_date INTEGER,
			track_count INTEGER
		);`)
	if err != nil {
		return fmt.Errorf("failed to create replacement albums table: %w", err)
	}

	userRatingExpr := "NULL"
	if columns["user_rating"] {
		userRatingExpr = "user_rating"
	} else if columns["rym_rating"] {
		userRatingExpr = "rym_rating"
	}

	selectQuery := fmt.Sprintf(
		"SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s FROM albums",
		sqlColumnOrDefault(columns, "id", "''"),
		sqlColumnOrDefault(columns, "title", "''"),
		sqlColumnOrDefault(columns, "artist", "''"),
		sqlColumnOrDefault(columns, "clean_title", "''"),
		sqlColumnOrDefault(columns, "clean_artist", "''"),
		sqlColumnOrDefault(columns, "genres", "NULL"),
		userRatingExpr,
		sqlColumnOrDefault(columns, "release_date", "NULL"),
		sqlColumnOrDefault(columns, "track_count", "NULL"),
	)

	rows, err := tx.Query(selectQuery)
	if err != nil {
		return fmt.Errorf("failed to read existing albums: %w", err)
	}
	defer rows.Close()

	insertStmt, err := tx.Prepare(`
		INSERT INTO albums_new (
			id, title, artist, clean_title, clean_artist, genres, user_rating, release_date, track_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insertStmt.Close()

	for rows.Next() {
		var (
			id          sql.NullString
			title       sql.NullString
			artist      sql.NullString
			cleanTitle  sql.NullString
			cleanArtist sql.NullString
			genres      sql.NullString
			userRating  sql.NullFloat64
			releaseDate sql.NullInt64
			trackCount  sql.NullInt64
		)

		if err := rows.Scan(&id, &title, &artist, &cleanTitle, &cleanArtist, &genres, &userRating, &releaseDate, &trackCount); err != nil {
			return err
		}

		record, err := normalizedAlbumRecord(id.String, title.String, artist.String, cleanTitle.String, cleanArtist.String)
		if err != nil {
			return err
		}

		if _, err := insertStmt.Exec(
			record.id,
			record.title,
			record.artist,
			record.cleanTitle,
			record.cleanArtist,
			nullableStringValue(genres),
			nullableFloatValue(userRating),
			nullableIntValue(releaseDate),
			nullableIntValue(trackCount),
		); err != nil {
			return fmt.Errorf("failed to migrate album %q: %w", record.id, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := insertStmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec("DROP TABLE albums"); err != nil {
		return fmt.Errorf("failed to drop old albums table: %w", err)
	}
	if _, err := tx.Exec("ALTER TABLE albums_new RENAME TO albums"); err != nil {
		return fmt.Errorf("failed to rename replacement albums table: %w", err)
	}

	return tx.Commit()
}

func rebuildTracksTable(db *sql.DB, columns map[string]bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		CREATE TABLE tracks_new (
			id TEXT PRIMARY KEY,
			album_id TEXT REFERENCES albums(id),
			title TEXT NOT NULL,
			album TEXT,
			artist TEXT NOT NULL,
			clean_title TEXT NOT NULL,
			clean_artist TEXT NOT NULL,
			genres TEXT,
			is_favorite INTEGER DEFAULT 0,
			is_disliked INTEGER DEFAULT 0
		);`)
	if err != nil {
		return fmt.Errorf("failed to create replacement tracks table: %w", err)
	}

	selectQuery := fmt.Sprintf(
		"SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s, %s FROM tracks",
		sqlColumnOrDefault(columns, "id", "''"),
		sqlColumnOrDefault(columns, "album_id", "NULL"),
		sqlColumnOrDefault(columns, "title", "''"),
		sqlColumnOrDefault(columns, "album", "NULL"),
		sqlColumnOrDefault(columns, "artist", "''"),
		sqlColumnOrDefault(columns, "clean_title", "''"),
		sqlColumnOrDefault(columns, "clean_artist", "''"),
		sqlColumnOrDefault(columns, "genres", "NULL"),
		sqlColumnOrDefault(columns, "is_favorite", "0"),
		sqlColumnOrDefault(columns, "is_disliked", "0"),
	)

	rows, err := tx.Query(selectQuery)
	if err != nil {
		return fmt.Errorf("failed to read existing tracks: %w", err)
	}
	defer rows.Close()

	insertStmt, err := tx.Prepare(`
		INSERT INTO tracks_new (
			id, album_id, title, album, artist, clean_title, clean_artist, genres, is_favorite, is_disliked
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insertStmt.Close()

	for rows.Next() {
		var (
			id          sql.NullString
			albumID     sql.NullString
			title       sql.NullString
			album       sql.NullString
			artist      sql.NullString
			cleanTitle  sql.NullString
			cleanArtist sql.NullString
			genres      sql.NullString
			isFavorite  sql.NullInt64
			isDisliked  sql.NullInt64
		)

		if err := rows.Scan(&id, &albumID, &title, &album, &artist, &cleanTitle, &cleanArtist, &genres, &isFavorite, &isDisliked); err != nil {
			return err
		}

		record, err := normalizedTrackRecord(id.String, title.String, artist.String, cleanTitle.String, cleanArtist.String)
		if err != nil {
			return err
		}

		favoriteFlag, dislikedFlag := affinityFlagValues(isFavorite, isDisliked)

		if _, err := insertStmt.Exec(
			record.id,
			nullableStringValue(albumID),
			record.title,
			nullableStringValue(album),
			record.artist,
			record.cleanTitle,
			record.cleanArtist,
			nullableStringValue(genres),
			favoriteFlag,
			dislikedFlag,
		); err != nil {
			return fmt.Errorf("failed to migrate track %q: %w", record.id, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := insertStmt.Close(); err != nil {
		return err
	}

	if _, err := tx.Exec("DROP TABLE tracks"); err != nil {
		return fmt.Errorf("failed to drop old tracks table: %w", err)
	}
	if _, err := tx.Exec("ALTER TABLE tracks_new RENAME TO tracks"); err != nil {
		return fmt.Errorf("failed to rename replacement tracks table: %w", err)
	}

	return tx.Commit()
}

func sqlColumnOrDefault(columns map[string]bool, column, fallback string) string {
	if columns[column] {
		return column
	}
	return fallback
}

type normalizedRecord struct {
	id          string
	title       string
	artist      string
	cleanTitle  string
	cleanArtist string
}

func normalizedAlbumRecord(id, title, artist, cleanTitle, cleanArtist string) (normalizedRecord, error) {
	return normalizedMediaRecord("album", id, title, artist, cleanTitle, cleanArtist)
}

func normalizedTrackRecord(id, title, artist, cleanTitle, cleanArtist string) (normalizedRecord, error) {
	return normalizedMediaRecord("track", id, title, artist, cleanTitle, cleanArtist)
}

func normalizedMediaRecord(label, id, title, artist, cleanTitle, cleanArtist string) (normalizedRecord, error) {
	title = strings.TrimSpace(title)
	artist = strings.TrimSpace(artist)
	cleanTitle = strings.TrimSpace(cleanTitle)
	cleanArtist = strings.TrimSpace(cleanArtist)

	var err error
	if cleanTitle == "" {
		cleanTitle, err = utils.NormalizeSearchText(title)
		if err != nil {
			return normalizedRecord{}, fmt.Errorf("failed to normalize %s title %q: %w", label, title, err)
		}
	}
	if cleanArtist == "" {
		cleanArtist, err = utils.NormalizeSearchText(artist)
		if err != nil {
			return normalizedRecord{}, fmt.Errorf("failed to normalize %s artist %q: %w", label, artist, err)
		}
	}

	return normalizedRecord{
		id:          strings.TrimSpace(id),
		title:       title,
		artist:      artist,
		cleanTitle:  cleanTitle,
		cleanArtist: cleanArtist,
	}, nil
}

func nullableStringValue(value sql.NullString) any {
	if value.Valid {
		return value.String
	}
	return nil
}

func nullableFloatValue(value sql.NullFloat64) any {
	if value.Valid {
		return value.Float64
	}
	return nil
}

func nullableIntValue(value sql.NullInt64) any {
	if value.Valid {
		return value.Int64
	}
	return nil
}

func favoriteFlagValue(value sql.NullInt64) int64 {
	if value.Valid && value.Int64 != 0 {
		return 1
	}
	return 0
}

func affinityFlagValues(isFavorite, isDisliked sql.NullInt64) (int64, int64) {
	favorite := favoriteFlagValue(isFavorite)
	disliked := favoriteFlagValue(isDisliked)
	if disliked == 1 {
		return 0, 1
	}
	if favorite == 1 {
		return 1, 0
	}
	return 0, 0
}

func refreshCleanSearchColumns(db *sql.DB, table string) error {
	query := fmt.Sprintf("SELECT id, title, artist FROM %s", table)
	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query %s rows for clean column refresh: %w", table, err)
	}

	var records []struct {
		id     string
		title  string
		artist string
	}
	for rows.Next() {
		var record struct {
			id     string
			title  string
			artist string
		}
		if err := rows.Scan(&record.id, &record.title, &record.artist); err != nil {
			rows.Close()
			return err
		}
		records = append(records, record)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	updateStmt, err := tx.Prepare(fmt.Sprintf("UPDATE %s SET clean_title = ?, clean_artist = ? WHERE id = ?", table))
	if err != nil {
		return err
	}
	defer updateStmt.Close()

	for _, record := range records {
		cleanTitle, err := utils.NormalizeSearchText(record.title)
		if err != nil {
			return fmt.Errorf("failed to normalize %s title %q: %w", table, record.title, err)
		}
		cleanArtist, err := utils.NormalizeSearchText(record.artist)
		if err != nil {
			return fmt.Errorf("failed to normalize %s artist %q: %w", table, record.artist, err)
		}

		if _, err := updateStmt.Exec(cleanTitle, cleanArtist, record.id); err != nil {
			return fmt.Errorf("failed to refresh %s clean columns for %q: %w", table, record.id, err)
		}
	}

	return tx.Commit()
}
