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

### Table: `recommendation_batches`
- `id` (TEXT, PRIMARY KEY)
- `prompt` (TEXT): Freeform recommendation request that generated the batch.
- `mood` (TEXT): Optional mood/context supplied by the user.
- `notes` (TEXT): Optional batch-level note.
- `created_at` (TEXT, DEFAULT CURRENT_TIMESTAMP)

### Table: `recommendation_candidates`
- `id` (TEXT, PRIMARY KEY)
- `batch_id` (TEXT, REFERENCES recommendation_batches(id))
- `artist` (TEXT, NOT NULL)
- `album` (TEXT, NOT NULL)
- `clean_artist` (TEXT, NOT NULL)
- `clean_title` (TEXT, NOT NULL)
- `starter_track` (TEXT): Matched track from discovery, used as the sample entry point for the album.
- `release_year` (INTEGER)
- `genre_tags` (TEXT): JSON-serialized string array from the verified discovery payload.
- `rank` (INTEGER): Position in the generated batch.
- `created_at` (TEXT, DEFAULT CURRENT_TIMESTAMP)

### Table: `recommendation_feedback`
- `id` (TEXT, PRIMARY KEY)
- `batch_id` (TEXT, REFERENCES recommendation_batches(id))
- `candidate_id` (TEXT, REFERENCES recommendation_candidates(id))
- `artist` (TEXT, NOT NULL)
- `album` (TEXT, NOT NULL)
- `clean_artist` (TEXT, NOT NULL)
- `clean_title` (TEXT, NOT NULL)
- `starter_track` (TEXT)
- `verdict` (TEXT, NOT NULL): One of `disliked`, `not_for_me_today`, `ok`, `good`, `great`, `already_know`.
- `mood` (TEXT): Optional listening context.
- `notes` (TEXT): Optional freeform reaction.
- `created_at` (TEXT, DEFAULT CURRENT_TIMESTAMP)

## Data Serialization Invariants
1. **Genres Array:** The `genres` column MUST always contain a valid JSON string array or remain `NULL`. Single string fallback values are not permitted.
2. **Taxonomy Order:** Subgenres within the array should be ordered by source confidence (highest upvoted tags first).
3. **Idempotent Flagging:** Ingestion of track-level interaction data must set `is_favorite = 1` permanently unless an explicit negative state change overrides it.
4. **Mutual Exclusion Principle:** A track record cannot simultaneously have `is_favorite = 1` and `is_disliked = 1`. If an incoming stream marks a track as disliked, `is_favorite` must be forced to 0, and vice versa.
5. **Recommendation Feedback Separation:** `recommendation_feedback.verdict` captures batch/listening reactions and MUST NOT be coerced into `albums.user_rating`. A formal 0-5 rating remains a separate album-critical score.
