## Purpose

Defines the analytical SQLite views that condense explicit personal curation markers — favorites, dislikes, and album evaluations — into high-fidelity taste profiles (artist affinity and genre topography), plus the profile CLI hook that renders them.

## Requirements

### Requirement: v_artist_affinity view
The system SHALL provide a `v_artist_affinity` view with columns `artist` (TEXT), `clean_artist` (TEXT), `favorite_tracks_count` (INTEGER, direct count where `tracks.is_favorite = 1`), `disliked_tracks_count` (INTEGER, direct count where `tracks.is_disliked = 1`), `avg_user_rating` (REAL, average score of the artist's rated albums), and `curved_affinity_score` (REAL, the principal metric used for ranking and taste anchoring).

The view MUST compute per-artist scores as follows: for each album, the base contribution is `COALESCE(albums.track_count, 0) + favorite_tracks_on_album` when `albums.user_rating >= 4.0`, otherwise `favorite_tracks_on_album`; each album contribution is clamped to `MIN(35.0, base contribution)`; and `curved_affinity_score = (SUM of clamped album contributions) - (disliked_tracks_count × 5.0)`. Default sorting MUST be strictly descending by `curved_affinity_score`.

#### Scenario: Masterpiece rating boosts contribution
- **WHEN** an album has `user_rating >= 4.0`
- **THEN** its base contribution adds `COALESCE(track_count, 0)` plus its favorite-track count

#### Scenario: Contribution capped at thirty-five
- **WHEN** an album's base contribution exceeds 35.0
- **THEN** the clamped contribution used in the sum is 35.0

#### Scenario: Disliked tracks penalize affinity
- **WHEN** an artist has disliked tracks
- **THEN** `curved_affinity_score` subtracts `disliked_tracks_count × 5.0`

#### Scenario: Artists sorted by affinity score
- **WHEN** `v_artist_affinity` is queried
- **THEN** rows are sorted strictly descending by `curved_affinity_score`

### Requirement: v_genre_topography view
The system SHALL provide a `v_genre_topography` view that flattens the JSON-serialized arrays stored in `tracks.genres` and `albums.genres` using SQLite's native `json_each` array modifier, exposing columns `subgenre` (TEXT), `total_tracks` (INTEGER), `favorite_tracks_count` (INTEGER), `disliked_tracks_count` (INTEGER), and `avg_album_rating` (REAL). Rows marked with track-level dislikes MUST be omitted from the active strength calculation profiles.

#### Scenario: Genre arrays flattened to rows
- **WHEN** a genre JSON array is stored on a track or album
- **THEN** `v_genre_topography` exposes each subgenre with its aggregated counts

#### Scenario: Disliked tracks excluded from strength
- **WHEN** a track has `is_disliked = 1`
- **THEN** it is omitted from the active strength calculation profiles

### Requirement: Profile CLI hook
The system MUST expose `go run ./cmd/music-vault profile`, which queries the analytical views and prints a clean, concise markdown text visualization directly to `stdout`: a markdown table of the Top 10 High-Affinity Artists based on the calculated `curved_affinity_score`, and a markdown table of the Top 5 Densest Subgenres sorted by explicit favorite counts.

#### Scenario: Profile renders top ten artists
- **WHEN** the user runs `go run ./cmd/music-vault profile`
- **THEN** a markdown table of the top 10 artists by `curved_affinity_score` is printed to `stdout`

#### Scenario: Profile renders top five genres
- **WHEN** the user runs `go run ./cmd/music-vault profile`
- **THEN** a markdown table of the top 5 densest subgenres sorted by favorite counts is printed to `stdout`
