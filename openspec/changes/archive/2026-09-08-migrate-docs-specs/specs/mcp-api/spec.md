## Purpose

Defines the Model Context Protocol (MCP) server surface the music library exposes to LLM clients over stdin/stdout JSON-RPC, including the registered tool contracts for library queries, ratings, and recommendation feedback.

## ADDED Requirements

### Requirement: MCP transport over stdin/stdout JSON-RPC
The communication layer MUST execute strictly via standard input/output (stdin/stdout) JSON-RPC messages utilizing the standard community Go SDK.

#### Scenario: Client exchanges JSON-RPC messages
- **WHEN** an MCP client communicates with the server
- **THEN** messages are exchanged over stdin/stdout as JSON-RPC

### Requirement: get_library_tracks tool
The `get_library_tracks` tool SHALL search the local music graph by track title or artist name. It requires a single argument: `query` (string) targeting song title or artist name.

#### Scenario: Search by title or artist fragment
- **WHEN** a client calls `get_library_tracks` with a `query`
- **THEN** matching track titles or artist names are returned

### Requirement: get_favorite_tracks tool
The `get_favorite_tracks` tool SHALL return a curated list of explicitly favorited songs (`is_favorite = 1`), filterable by a genre keyword. Inputs: `genre` (string, optional) a specific subgenre keyword such as "sludge" or "progressive metal", and `limit` (integer, optional) maximum records to return, defaulting to 50.

#### Scenario: Favorite tracks filtered by genre
- **WHEN** a client calls `get_favorite_tracks` with `genre` "sludge"
- **THEN** only favorited tracks matching the genre are returned, up to the default limit of 50

### Requirement: get_top_rated_albums tool
The `get_top_rated_albums` tool SHALL retrieve the highest-evaluated albums ordered strictly by critical score, filterable by genre. Inputs: `min_rating` (real, optional) minimum critical score cutoff on a 5.0 scale, defaulting to 4.0, and `genre` (string, optional) a subgenre keyword to filter the album matrix.

#### Scenario: Albums above rating cutoff returned
- **WHEN** a client calls `get_top_rated_albums`
- **THEN** albums with `user_rating` at or above the `min_rating` cutoff (default 4.0) are returned ordered strictly by score

### Requirement: get_genre_distribution tool
The `get_genre_distribution` tool SHALL inspect the macro-topography of the music graph and return a summarized list of all deduplicated subgenres present in the database, ordered by frequency count. It takes no arguments and MUST render a Markdown table with `Subgenre` and `Total Occurrences`.

#### Scenario: Genre distribution rendered as Markdown table
- **WHEN** a client calls `get_genre_distribution`
- **THEN** a Markdown table of deduplicated subgenres and their occurrence counts, ordered by frequency, is returned

### Requirement: get_album_tracks tool
The `get_album_tracks` tool SHALL retrieve the complete structured tracklist of a specific album. Inputs: `artist` (string, required) and `album` (string, required).

#### Scenario: Tracklist for named album
- **WHEN** a client calls `get_album_tracks` with `artist` and `album`
- **THEN** the complete structured tracklist for that album is returned

### Requirement: get_taste_adjacencies tool
The `get_taste_adjacencies` tool SHALL return local artist affinity and micro-genre topography context for grounded discovery and recommendation workflows. Inputs: `seed_artists` (array of strings, optional) artist names to build outward from, defaulting to the user's top high-affinity artists when omitted, and `target_vibe` (string, optional) a sonic, technical, or mood descriptor to guide discovery.

#### Scenario: Defaults to high-affinity artists
- **WHEN** a client calls `get_taste_adjacencies` without `seed_artists`
- **THEN** context is built from the user's top high-affinity artists

### Requirement: get_verified_discovery_candidates tool
The `get_verified_discovery_candidates` tool SHALL search live MusicBrainz metadata using canonical genre tags and exclude exact albums already rated or blocked through recommendation feedback. Known artists and track-level history SHALL NOT be global exclusions. Abstract vibes MUST be translated to semantic fallback tags before the external request. Inputs: `target_vibe` (string, optional) a raw vibe, canonical MusicBrainz genre tag, or comma-separated canonical tag list; `fallback_tags` (array of strings, optional) canonical MusicBrainz genre tags derived from an abstract phrase (e.g., `"erratic rhythm section"` becomes `["math rock", "idm", "breakcore"]`); `limit` (integer, optional) maximum candidates, defaulting to 5 with a maximum of 50. At least one of `target_vibe` or `fallback_tags` is required, and when both are supplied `fallback_tags` takes precedence. The tool SHALL return a JSON object containing `instructions`, `effective_limit` (the validated limit after max-cap enforcement), and `candidates` (verified real-world recording metadata with album fields). Recommendation clients MUST present candidates as album recommendations by default and use `track_name` as the matched track/sample entry point.

#### Scenario: Fallback tags take precedence
- **WHEN** both `target_vibe` and `fallback_tags` are supplied
- **THEN** `fallback_tags` drive the MusicBrainz query

#### Scenario: Limit capped at fifty
- **WHEN** a client supplies a `limit` greater than 50
- **THEN** the `effective_limit` is capped at 50 and reported in the output envelope

#### Scenario: Missing both vibe inputs rejected
- **WHEN** a client supplies neither `target_vibe` nor `fallback_tags`
- **THEN** the call is rejected as invalid

### Requirement: log_album_rating tool
The `log_album_rating` tool SHALL persist a local album rating to `albums.user_rating` using the clean key normalization rule from the string-normalization capability, inserting a UUID-backed album row if absent. Inputs: `artist` (string, required), `album` (string, required), and `rating` (real, required) the personal score on the local 0.0 to 5.0 scale.

#### Scenario: Rating persisted to existing album
- **WHEN** a client calls `log_album_rating` for an existing album
- **THEN** `albums.user_rating` is updated using the normalized clean album key

#### Scenario: Missing album row inserted
- **WHEN** a client calls `log_album_rating` for an album with no existing row
- **THEN** a UUID-backed album row is inserted with the rating

### Requirement: log_recommendation_feedback tool
The `log_recommendation_feedback` tool SHALL persist album-first recommendation feedback without converting it into a formal 0-5 album rating. Inputs: `artist` (string, required), `album` (string, required), `verdict` (string, required) one of `disliked`, `not_for_me_today`, `ok`, `good`, `great`, `already_know`, with natural aliases like `it's ok` and `not today` accepted; `starter_track` (string, optional) matched track/sample entry point; `batch_id` (string, optional); `candidate_id` (string, optional); `mood` (string, optional) listening context; `notes` (string, optional) freeform reaction. Durable verdicts SHALL exclude the exact album from future discovery; `not_for_me_today` SHALL exclude the exact album only on the same local calendar day.

#### Scenario: Verdict aliases normalized to canonical value
- **WHEN** a client supplies the verdict alias "not today"
- **THEN** the alias is accepted and stored as the canonical verdict `not_for_me_today`

#### Scenario: Feedback recorded without rating change
- **WHEN** a client calls `log_recommendation_feedback`
- **THEN** a feedback row is stored and `albums.user_rating` is not modified
