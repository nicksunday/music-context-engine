## Purpose

Defines the relational SQLite V1 schema underpinning the local music library: the albums, tracks, recommendation batches, candidates, and feedback tables plus the data serialization invariants that keep multi-platform ingestion consistent.

## ADDED Requirements

### Requirement: Albums table schema
The system SHALL maintain an `albums` table with the following columns:

- `id` (TEXT, PRIMARY KEY)
- `title` (TEXT, NOT NULL)
- `artist` (TEXT, NOT NULL)
- `clean_title` (TEXT, NOT NULL) — lowercase, stripped of all diacritical marks/accents for index searching.
- `clean_artist` (TEXT, NOT NULL) — lowercase, stripped of all diacritical marks/accents for index searching.
- `genres` (TEXT) — JSON-serialized string array of deduplicated subgenres.
- `user_rating` (REAL) — personal critical score scaled to a maximum of 5.0.
- `release_date` (INTEGER) — epoch timestamp or year integer.
- `track_count` (INTEGER) — total physical track count of the album release, populated via the metadata enrichment engine.

#### Scenario: Album requires identity fields
- **WHEN** an album row is inserted without `title` or `artist`
- **THEN** the insert is rejected and no partial album row is persisted

### Requirement: Tracks table schema
The system SHALL maintain a `tracks` table with the following columns:

- `id` (TEXT, PRIMARY KEY)
- `album_id` (TEXT, REFERENCES albums(id))
- `title` (TEXT, NOT NULL)
- `album` (TEXT)
- `artist` (TEXT, NOT NULL)
- `clean_title` (TEXT, NOT NULL) — lowercase, stripped of all diacritical marks/accents for index searching.
- `clean_artist` (TEXT, NOT NULL) — lowercase, stripped of all diacritical marks/accents for index searching.
- `genres` (TEXT) — JSON-serialized string array of deduplicated subgenres mirroring parent/artist metadata.
- `is_favorite` (INTEGER, DEFAULT 0) — binary boolean flag (0 or 1); unified indicator of explicit positive personal validation (YouTube Music Likes, Last.fm Loved, Apple Music Likes/Favorites).
- `is_disliked` (INTEGER, DEFAULT 0) — binary boolean flag (0 or 1); dedicated explicit suppression vector captured via native streaming platform negative actions (e.g., Apple Music "DISLIKE" statuses). Mutually exclusive with `is_favorite`.

#### Scenario: Track references an album
- **WHEN** a track row is inserted with an `album_id`
- **THEN** the `album_id` MUST reference an existing row in the `albums` table

### Requirement: Recommendation batches table schema
The system SHALL maintain a `recommendation_batches` table with the following columns: `id` (TEXT, PRIMARY KEY), `prompt` (TEXT) storing the freeform recommendation request that generated the batch, `mood` (TEXT) optional mood/context supplied by the user, `notes` (TEXT) optional batch-level note, and `created_at` (TEXT, DEFAULT CURRENT_TIMESTAMP).

#### Scenario: New batch receives default timestamp
- **WHEN** a recommendation batch is created without an explicit `created_at`
- **THEN** `created_at` defaults to the current timestamp

### Requirement: Recommendation candidates table schema
The system SHALL maintain a `recommendation_candidates` table with the following columns: `id` (TEXT, PRIMARY KEY), `batch_id` (TEXT, REFERENCES recommendation_batches(id)), `artist` (TEXT, NOT NULL), `album` (TEXT, NOT NULL), `clean_artist` (TEXT, NOT NULL), `clean_title` (TEXT, NOT NULL), `starter_track` (TEXT) storing the matched track from discovery used as the sample entry point, `release_year` (INTEGER), `genre_tags` (TEXT) JSON-serialized string array from the verified discovery payload, `rank` (INTEGER) position in the generated batch, and `created_at` (TEXT, DEFAULT CURRENT_TIMESTAMP).

#### Scenario: Candidate references its batch
- **WHEN** a candidate row is inserted with a `batch_id`
- **THEN** the `batch_id` MUST reference an existing row in the `recommendation_batches` table

### Requirement: Recommendation feedback table schema
The system SHALL maintain a `recommendation_feedback` table with the following columns: `id` (TEXT, PRIMARY KEY), `batch_id` (TEXT, REFERENCES recommendation_batches(id)), `candidate_id` (TEXT, REFERENCES recommendation_candidates(id)), `artist` (TEXT, NOT NULL), `album` (TEXT, NOT NULL), `clean_artist` (TEXT, NOT NULL), `clean_title` (TEXT, NOT NULL), `starter_track` (TEXT), `verdict` (TEXT, NOT NULL) restricted to one of `disliked`, `not_for_me_today`, `ok`, `good`, `great`, `already_know`, `mood` (TEXT) optional listening context, `notes` (TEXT) optional freeform reaction, and `created_at` (TEXT, DEFAULT CURRENT_TIMESTAMP).

#### Scenario: Feedback verdict is constrained
- **WHEN** a feedback row is inserted
- **THEN** `verdict` MUST be one of `disliked`, `not_for_me_today`, `ok`, `good`, `great`, or `already_know`

### Requirement: Genres array serialization invariant
The `genres` column on the `albums` and `tracks` tables MUST always contain a valid JSON string array or remain `NULL`. Single string fallback values MUST NOT be stored.

#### Scenario: Genres stored as JSON array
- **WHEN** a genres value is persisted on an album or track
- **THEN** it is stored as a valid JSON-serialized string array or remains `NULL`

#### Scenario: Single string genres value not stored
- **WHEN** a bare non-array string is written to a genres column
- **THEN** the bare string is not persisted as the column value

### Requirement: Genre taxonomy ordering
Subgenres within the genres array SHALL be ordered by source confidence, with the highest upvoted tags first.

#### Scenario: Subgenres ordered by source confidence
- **WHEN** a subgenre array is written
- **THEN** the elements appear in descending source-confidence order (highest upvoted tags first)

### Requirement: Idempotent favorite flagging
Ingestion of track-level interaction data MUST set `is_favorite = 1` permanently unless an explicit negative state change overrides it.

#### Scenario: Favorite persists across repeated ingestion
- **WHEN** a track is marked favorite by an ingestion source more than once
- **THEN** `is_favorite` remains 1 and no side effects accumulate

#### Scenario: Negative state overrides favorite
- **WHEN** an explicit negative state change (`is_disliked = 1`) is applied to a favorited track
- **THEN** `is_favorite` is forced to 0

### Requirement: Mutual exclusion of favorite and dislike
A track record MUST NOT simultaneously have `is_favorite = 1` and `is_disliked = 1`. If an incoming stream marks a track as disliked, `is_favorite` MUST be forced to 0, and vice versa.

#### Scenario: Dislike clears favorite
- **WHEN** an incoming stream marks a track as disliked
- **THEN** `is_disliked` is set to 1 and `is_favorite` is forced to 0

#### Scenario: Favorite clears dislike
- **WHEN** an incoming stream marks a track as favorite
- **THEN** `is_favorite` is set to 1 and `is_disliked` is forced to 0

### Requirement: Recommendation feedback separation
`recommendation_feedback.verdict` captures batch/listening reactions and MUST NOT be coerced into `albums.user_rating`. A formal 0-5 rating remains a separate album-critical score.

#### Scenario: Feedback never writes a user rating
- **WHEN** recommendation feedback is logged
- **THEN** no write is made to `albums.user_rating` as a result of that feedback
