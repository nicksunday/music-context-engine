## Purpose

Defines genre-filtered profile queries that let the recommendation engine request top affinity artists for a specific micro-genre directly from the CLI, offloading genre-to-artist matching onto SQLite's relational layer.

## ADDED Requirements

### Requirement: Profile CLI flag interface
The profiling utility MUST accept the command syntax `go run ./cmd/music-vault profile [--genre <string>] [--limit <int>]`. With no flags it MUST fall back to the default global Top 10 layout established by the advanced-analytics capability. With `--genre` specified, it MUST filter output to display only artists who have tracks or albums matching that genre string (case-insensitive). The `--limit` flag MUST control the max rows returned, defaulting to 10 when not specified.

#### Scenario: No flags uses default top ten
- **WHEN** the user runs `music-vault profile` with no flags
- **THEN** the default global Top 10 layout is produced

#### Scenario: Genre filter is case-insensitive
- **WHEN** the user runs `music-vault profile --genre "Death Metal"`
- **THEN** only artists matching "death metal" (case-insensitive) are returned

#### Scenario: Limit defaults to ten
- **WHEN** the user runs `music-vault profile --genre "sludge"` without `--limit`
- **THEN** at most 10 rows are returned

### Requirement: Relational genre resolution
To fulfill a genre-filtered query, the Go database layer MUST cross-reference the artist tables with the flattened arrays processed by `v_genre_topography`: the query MUST safely match the user-supplied string against the unpacked strings in the `tracks.genres` or `albums.genres` JSON arrays for each artist. The result set MUST mirror the core affinity layout — `Artist`, `Favorite Tracks`, `Avg RYM`, `Score` — sorted strictly descending by `composite_score`.

#### Scenario: Genre join against flattened arrays
- **WHEN** a genre-filtered profile query runs
- **THEN** artists are matched via the unpacked genre arrays from `v_genre_topography`

#### Scenario: Results sorted by composite score
- **WHEN** a genre-filtered profile query runs
- **THEN** rows show `Artist`, `Favorite Tracks`, `Avg RYM`, and `Score` sorted strictly descending by `composite_score`

### Requirement: Example genre-filtered workflow
When invoked as `music-vault profile --genre "death metal" --limit 5`, the utility MUST produce a markdown table titled `Top 5 Affinity Artists for Genre: "death metal"` with columns `Rank`, `Artist`, `Favorite Tracks`, `Avg RYM`, and `Score`.

#### Scenario: Death metal profile output
- **WHEN** the user runs `music-vault profile --genre "death metal" --limit 5`
- **THEN** a "Top 5 Affinity Artists for Genre" markdown table with the affinity columns is produced
