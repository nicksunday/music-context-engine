# Specification 01: Relational Data Schema (SQLite V1)

## 1. Entities & Constraints

### Table: `albums`
- `id` (TEXT, PRIMARY KEY)
- `title` (TEXT, NOT NULL)
- `artist` (TEXT, NOT NULL)
- `clean_title` (TEXT, NOT NULL): Lowercase, stripped of all diacritical marks/accents for index searching.
- `clean_artist` (TEXT, NOT NULL): Lowercase, stripped of all diacritical marks/accents for index searching.
- `genres` (TEXT): JSON-serialized string array of deduplicated subgenres.
- `user_rating` (REAL): Personal critical score scaled to a maximum of 5.0.
- `release_date` (INTEGER): Epoch timestamp or year integer.
- `track_count` (INTEGER): Total physical track count of the album release, populated via the Metadata Enrichment Engine.

### Table: `tracks`
- `id` (TEXT, PRIMARY KEY)
- `album_id` (TEXT, REFERENCES albums(id))
- `title` (TEXT, NOT NULL)
- `album` (TEXT)
- `artist` (TEXT, NOT NULL)
- `clean_title` (TEXT, NOT NULL): Lowercase, stripped of all diacritical marks/accents for index searching.
- `clean_artist` (TEXT, NOT NULL): Lowercase, stripped of all diacritical marks/accents for index searching.
- `genres` (TEXT): JSON-serialized string array of deduplicated subgenres mirroring parent/artist metadata.
- `is_favorite` (INTEGER, DEFAULT 0): Binary boolean flag (0 or 1). Unified indicator of explicit positive personal validation (YTM Likes, Last.fm Loved, Apple Music Likes/Favorites).
- `is_disliked` (INTEGER, DEFAULT 0): Binary boolean flag (0 or 1). Dedicated explicit suppression vector captured via native streaming platform negative actions (e.g., Apple Music "DISLIKE" statuses). Mutually exclusive with `is_favorite`.

## Data Serialization Invariants
1. **Genres Array:** The `genres` column MUST always contain a valid JSON string array or remain `NULL`. Single string fallback values are not permitted.
2. **Taxonomy Order:** Subgenres within the array should be ordered by source confidence (highest upvoted tags first).
3. **Idempotent Flagging:** Ingestion of track-level interaction data must set `is_favorite = 1` permanently unless an explicit negative state change overrides it.
4. **Mutual Exclusion Principle:** A track record cannot simultaneously have `is_favorite = 1` and `is_disliked = 1`. If an incoming stream marks a track as disliked, `is_favorite` must be forced to 0, and vice versa.