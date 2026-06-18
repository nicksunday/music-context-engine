package database

import (
	"context"
	"database/sql"
	"fmt"
)

type ArtistAffinity struct {
	Artist              string
	CleanArtist         string
	FavoriteTracksCount int
	DislikedTracksCount int
	AvgUserRating       sql.NullFloat64
	CurvedAffinityScore float64
}

type GenreTopography struct {
	Subgenre            string
	TotalTracks         int
	FavoriteTracksCount int
	DislikedTracksCount int
	AvgAlbumRating      sql.NullFloat64
}

func dropAnalyticalViews(db *sql.DB) error {
	_, err := db.Exec(`
		DROP VIEW IF EXISTS v_artist_affinity;
		DROP VIEW IF EXISTS v_genre_topography;
	`)
	if err != nil {
		return fmt.Errorf("failed to drop analytical views: %w", err)
	}
	return nil
}

func ensureAnalyticalViews(db *sql.DB) error {
	_, err := db.Exec(`
		DROP VIEW IF EXISTS v_artist_affinity;
		DROP VIEW IF EXISTS v_genre_topography;

		CREATE VIEW v_artist_affinity AS
		WITH album_track_counts AS (
			SELECT
				albums.id AS album_id,
				albums.artist AS artist,
				albums.clean_artist AS clean_artist,
				albums.user_rating AS user_rating,
				COALESCE(albums.track_count, 0) AS track_count,
				COALESCE(SUM(CASE WHEN COALESCE(tracks.is_favorite, 0) = 1 THEN 1 ELSE 0 END), 0) AS fav_tracks_on_album,
				COALESCE(SUM(CASE WHEN COALESCE(tracks.is_disliked, 0) = 1 THEN 1 ELSE 0 END), 0) AS disliked_tracks_on_album
			FROM albums
			LEFT JOIN tracks ON tracks.album_id = albums.id
			WHERE trim(COALESCE(albums.clean_artist, '')) != ''
			GROUP BY albums.id, albums.artist, albums.clean_artist, albums.user_rating, albums.track_count
		),
		album_contributions AS (
			SELECT
				artist,
				clean_artist,
				fav_tracks_on_album AS favorite_tracks_count,
				disliked_tracks_on_album AS disliked_tracks_count,
				user_rating,
				MIN(
					35.0,
					CASE
						WHEN user_rating >= 4.0 THEN track_count + fav_tracks_on_album
						ELSE fav_tracks_on_album
					END
				) AS curved_affinity_score
			FROM album_track_counts
		),
		unmatched_track_counts AS (
			SELECT
				MIN(tracks.artist) AS artist,
				tracks.clean_artist AS clean_artist,
				COALESCE(SUM(CASE WHEN COALESCE(tracks.is_favorite, 0) = 1 THEN 1 ELSE 0 END), 0) AS favorite_tracks_count,
				COALESCE(SUM(CASE WHEN COALESCE(tracks.is_disliked, 0) = 1 THEN 1 ELSE 0 END), 0) AS disliked_tracks_count,
				NULL AS user_rating,
				COALESCE(SUM(CASE WHEN COALESCE(tracks.is_favorite, 0) = 1 THEN 1 ELSE 0 END), 0) AS curved_affinity_score
			FROM tracks
			LEFT JOIN albums ON albums.id = tracks.album_id
			WHERE (COALESCE(tracks.is_favorite, 0) = 1 OR COALESCE(tracks.is_disliked, 0) = 1)
				AND trim(COALESCE(tracks.clean_artist, '')) != ''
				AND albums.id IS NULL
			GROUP BY tracks.clean_artist
		),
		affinity_contributions AS (
			SELECT artist, clean_artist, favorite_tracks_count, disliked_tracks_count, user_rating, curved_affinity_score
			FROM album_contributions
			UNION ALL
			SELECT artist, clean_artist, favorite_tracks_count, disliked_tracks_count, user_rating, curved_affinity_score
			FROM unmatched_track_counts
		)
		SELECT
			MIN(artist) AS artist,
			clean_artist,
			SUM(favorite_tracks_count) AS favorite_tracks_count,
			SUM(disliked_tracks_count) AS disliked_tracks_count,
			AVG(user_rating) AS avg_user_rating,
			SUM(curved_affinity_score) - (SUM(disliked_tracks_count) * 5.0) AS curved_affinity_score
		FROM affinity_contributions
		GROUP BY clean_artist
		HAVING SUM(favorite_tracks_count) > 0
			OR SUM(disliked_tracks_count) > 0
			OR AVG(user_rating) IS NOT NULL
		ORDER BY curved_affinity_score DESC, favorite_tracks_count DESC, artist COLLATE NOCASE ASC;

		CREATE VIEW v_genre_topography AS
		WITH raw_track_genres AS (
			SELECT
				lower(trim(CAST(track_genre.value AS TEXT))) AS subgenre,
				tracks.id AS track_id,
				CASE WHEN COALESCE(tracks.is_favorite, 0) = 1 THEN 1 ELSE 0 END AS is_favorite,
				CASE WHEN COALESCE(tracks.is_disliked, 0) = 1 THEN 1 ELSE 0 END AS is_disliked
			FROM tracks,
				json_each(CASE WHEN json_valid(tracks.genres) THEN tracks.genres ELSE '[]' END) AS track_genre
			WHERE trim(CAST(track_genre.value AS TEXT)) != ''
		),
		track_genres AS (
			SELECT
				subgenre,
				track_id,
				MAX(is_favorite) AS is_favorite,
				MAX(is_disliked) AS is_disliked
			FROM raw_track_genres
			GROUP BY subgenre, track_id
		),
		track_counts AS (
			SELECT
				subgenre,
				COALESCE(SUM(CASE WHEN is_disliked = 0 THEN 1 ELSE 0 END), 0) AS total_tracks,
				COALESCE(SUM(CASE WHEN is_disliked = 0 THEN is_favorite ELSE 0 END), 0) AS favorite_tracks_count,
				COALESCE(SUM(is_disliked), 0) AS disliked_tracks_count
			FROM track_genres
			GROUP BY subgenre
		),
		raw_album_genres AS (
			SELECT
				lower(trim(CAST(album_genre.value AS TEXT))) AS subgenre,
				albums.id AS album_id,
				albums.user_rating AS user_rating
			FROM albums,
				json_each(CASE WHEN json_valid(albums.genres) THEN albums.genres ELSE '[]' END) AS album_genre
			WHERE trim(CAST(album_genre.value AS TEXT)) != ''
		),
		album_genres AS (
			SELECT subgenre, album_id, MAX(user_rating) AS user_rating
			FROM raw_album_genres
			GROUP BY subgenre, album_id
		),
		album_ratings AS (
			SELECT subgenre, AVG(user_rating) AS avg_album_rating
			FROM album_genres
			WHERE user_rating IS NOT NULL
			GROUP BY subgenre
		),
		genre_index AS (
			SELECT subgenre FROM track_counts
			UNION
			SELECT subgenre FROM album_genres
		)
		SELECT
			genre_index.subgenre AS subgenre,
			COALESCE(track_counts.total_tracks, 0) AS total_tracks,
			COALESCE(track_counts.favorite_tracks_count, 0) AS favorite_tracks_count,
			COALESCE(track_counts.disliked_tracks_count, 0) AS disliked_tracks_count,
			album_ratings.avg_album_rating AS avg_album_rating
		FROM genre_index
		LEFT JOIN track_counts ON track_counts.subgenre = genre_index.subgenre
		LEFT JOIN album_ratings ON album_ratings.subgenre = genre_index.subgenre
		ORDER BY favorite_tracks_count DESC, total_tracks DESC, subgenre COLLATE NOCASE ASC;
	`)
	if err != nil {
		return fmt.Errorf("failed to ensure analytical views: %w", err)
	}
	return nil
}

func FetchTopArtistAffinities(ctx context.Context, db *sql.DB, limit int) ([]ArtistAffinity, error) {
	const stmt = `
		SELECT artist, clean_artist, favorite_tracks_count, disliked_tracks_count, avg_user_rating, curved_affinity_score
		FROM v_artist_affinity
		ORDER BY curved_affinity_score DESC, favorite_tracks_count DESC, artist COLLATE NOCASE ASC
		LIMIT ?`

	return queryArtistAffinities(ctx, db, stmt, limit)
}

func FetchTopArtistAffinitiesByGenre(ctx context.Context, db *sql.DB, genre string, limit int) ([]ArtistAffinity, error) {
	const stmt = `
		WITH genre_artists AS (
			SELECT DISTINCT tracks.clean_artist
			FROM tracks,
				json_each(CASE WHEN json_valid(tracks.genres) THEN tracks.genres ELSE '[]' END) AS track_genre
			WHERE trim(COALESCE(tracks.clean_artist, '')) != ''
				AND lower(trim(CAST(track_genre.value AS TEXT))) = lower(trim(?))
			UNION
			SELECT DISTINCT albums.clean_artist
			FROM albums,
				json_each(CASE WHEN json_valid(albums.genres) THEN albums.genres ELSE '[]' END) AS album_genre
			WHERE trim(COALESCE(albums.clean_artist, '')) != ''
				AND lower(trim(CAST(album_genre.value AS TEXT))) = lower(trim(?))
		)
		SELECT
			affinity.artist,
			affinity.clean_artist,
			affinity.favorite_tracks_count,
			affinity.disliked_tracks_count,
			affinity.avg_user_rating,
			affinity.curved_affinity_score
		FROM v_artist_affinity AS affinity
		JOIN genre_artists ON genre_artists.clean_artist = affinity.clean_artist
		ORDER BY affinity.curved_affinity_score DESC, affinity.favorite_tracks_count DESC, affinity.artist COLLATE NOCASE ASC
		LIMIT ?`

	return queryArtistAffinities(ctx, db, stmt, genre, genre, limit)
}

func queryArtistAffinities(ctx context.Context, db *sql.DB, stmt string, args ...any) ([]ArtistAffinity, error) {
	rows, err := db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var affinities []ArtistAffinity
	for rows.Next() {
		var affinity ArtistAffinity
		if err := rows.Scan(
			&affinity.Artist,
			&affinity.CleanArtist,
			&affinity.FavoriteTracksCount,
			&affinity.DislikedTracksCount,
			&affinity.AvgUserRating,
			&affinity.CurvedAffinityScore,
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

func FetchTopGenreTopography(ctx context.Context, db *sql.DB, limit int) ([]GenreTopography, error) {
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

	var topography []GenreTopography
	for rows.Next() {
		var genre GenreTopography
		if err := rows.Scan(
			&genre.Subgenre,
			&genre.TotalTracks,
			&genre.FavoriteTracksCount,
			&genre.DislikedTracksCount,
			&genre.AvgAlbumRating,
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
