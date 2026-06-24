# Specification 03: Model Context Protocol (MCP) API Interface

## 1. Transport Requirements
The communication layer must execute strictly via standard input/output (`stdin`/`stdout`) JSON-RPC messages utilizing the standard community Go SDK.

## 2. Registered Capabilities

### Tool: `get_library_tracks`
- **Description:** Search your complete local music graph by track title or artist name.
- **Input Arguments:**
  - `query` (string, required): Text fragment targeting song title or artist name.

### Tool: `get_favorite_tracks`
- **Description:** Pull a curated list of explicitly favorited songs (`is_favorite = 1`), filterable by a specific genre keyword.
- **Input Arguments:**
  - `genre` (string, optional): A specific subgenre keyword to filter by (e.g., "sludge", "progressive metal").
  - `limit` (integer, optional): Maximum records to return (Default: 50).

### Tool: `get_top_rated_albums`
- **Description:** Retrieve your highest-evaluated albums ordered strictly by critical score, filterable by genre.
- **Input Arguments:**
  - `min_rating` (real, optional): Minimum critical score cutoff on a 5.0 scale (Default: 4.0).
  - `genre` (string, optional): A specific subgenre keyword to filter the album matrix by.

### Tool: `get_genre_distribution`
- **Description:** Inspect the macro-topography of the music graph. Returns a summarized list of all deduplicated subgenres present in the database, ordered by frequency count.
- **Input Arguments:** None.
- **Output:** A clean Markdown table displaying `Subgenre` and `Total Occurrences`.

### Tool: `get_album_tracks`
- **Description:** Retrieve the complete structured tracklist of a specific album.
- **Input Arguments:**
  - `artist` (string, required): The artist name.
  - `album` (string, required): The album title.

### Tool: `get_taste_adjacencies`
- **Description:** Return local artist affinity and micro-genre topography context for grounded discovery and recommendation workflows.
- **Input Arguments:**
  - `seed_artists` (array of strings, optional): Artist names to build outward from. If omitted, defaults to the user's top high-affinity artists.
  - `target_vibe` (string, optional): A sonic, technical, or mood descriptor to guide discovery.

### Tool: `get_verified_discovery_candidates`
- **Description:** Search live MusicBrainz metadata using canonical genre tags and exclude candidates already represented in the local library. Abstract vibes must be translated to semantic fallback tags before the external request.
- **Input Arguments:**
  - `target_vibe` (string, optional): A raw vibe, canonical MusicBrainz genre tag, or comma-separated canonical tag list.
  - `fallback_tags` (array of strings, optional): Canonical MusicBrainz genre tags derived from an abstract phrase. For example, `"erratic rhythm section"` becomes `["math rock", "idm", "breakcore"]`.
  - `limit` (integer, optional): Maximum candidates to return (Default: 5; maximum: 50).
- **Validation:** At least one of `target_vibe` or `fallback_tags` is required. When both are supplied, `fallback_tags` takes precedence.
- **Output:** A JSON object containing `instructions` (critical recommendation output contract), `effective_limit` (the validated limit after max-cap enforcement), and `candidates` (verified real-world track metadata).

### Tool: `log_album_rating`
- **Description:** Persist a local album rating to `albums.user_rating` using Specification 07 clean key normalization; insert a UUID-backed album row if absent.
- **Input Arguments:**
  - `artist` (string, required): The album artist name.
  - `album` (string, required): The album title.
  - `rating` (real, required): Personal score on the local 0.0 to 5.0 scale.
