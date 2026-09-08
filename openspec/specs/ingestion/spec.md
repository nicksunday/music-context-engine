## Purpose

Defines how the ingestion pipeline detects each source format by content sniffing, maps each platform's export into the library schema with affinity and suppression vectors, filters ambient noise telemetry, and normalizes text for cross-platform matching.

## Requirements

### Requirement: Dynamic source detection by content sniffing
The ingestion engine MUST determine the processing routine dynamically by inspect-reading the first record (header row) of any incoming file argument, not by matching file name suffixes or paths.

#### Scenario: Last.fm layout detected by headers
- **WHEN** an input file's header row contains `uts`, `track_mbid`, and `artist`
- **THEN** the Last.fm processing routine is selected

#### Scenario: YouTube Music uploads layout detected by headers
- **WHEN** an input file's header row contains `Song Title` and `Artist Name 1`
- **THEN** the YouTube Music uploads processing routine is selected

#### Scenario: RateYourMusic layout detected by headers
- **WHEN** an input file's header row contains `RYM Album ID` and `Rating`
- **THEN** the RateYourMusic processing routine is selected

#### Scenario: Apple Music likes CSV detected by headers
- **WHEN** an input file's first four header fields are `Name`, `Artist`, `Composer`, `Album`
- **THEN** the Apple Music likes CSV processing routine is selected

#### Scenario: Apple Music library JSON detected by shape
- **WHEN** an input is a top-level JSON array whose root objects contain `Track Identifier`, `Title`, and `Content Type`
- **THEN** the Apple Music library track JSON processing routine is selected

#### Scenario: Apple Music play activity detected by headers
- **WHEN** an input file's header row contains `Event Timestamp`, `Track Description`, and `End Reason Type`
- **THEN** the Apple Music play activity processing routine is selected

### Requirement: YouTube Music streaming history contract
CSV arrays from YouTube Music streaming history MUST map directly to 3 indices without matching standard headers: `record[1]` to `title`, `record[2]` to `album`, and `record[3]` to `artist`. Ingestion MUST execute an idempotent update setting `is_favorite = 1` on the resolved track record.

#### Scenario: Streaming history row marks favorite
- **WHEN** a YouTube Music streaming history record is ingested
- **THEN** title, album, and artist are read from indices 1, 2, and 3 and the resolved track is set `is_favorite = 1`

### Requirement: YouTube Music uploads contract
For files matching headers `Song Title` and `Artist Name 1`, ingestion MUST map `Song Title` to `title`, `Album Title` to `album`, and `Artist Name 1` to `artist`, falling back to "Unknown Artist" when the artist is empty.

#### Scenario: Uploads row with blank artist falls back
- **WHEN** `Artist Name 1` is empty in a YouTube Music uploads row
- **THEN** the artist is recorded as "Unknown Artist"

### Requirement: Last.fm history contract
Ingestion MUST reconcile Last.fm rows against existing `tracks` and `albums` data using the text normalization invariants. On a match, ingestion MUST execute an idempotent update setting `is_favorite = 1` without altering existing album relationships. On no match, unmatched rows MUST be inserted as new records rather than skipped: attempt to look up or insert a baseline album using normalized matching when a non-empty `album` string exists, map to a generic shared "Unknown Album" record under that artist when the album field is missing or empty, then insert the track with `is_favorite = 1` linked to the resolved `album_id`. Ingestion MUST NOT create time-series log entries, record historical timestamps, or increment play count integers.

#### Scenario: Last.fm row matches existing track
- **WHEN** a Last.fm row normalizes to an existing track
- **THEN** `is_favorite` is set to 1 and no album relationship is altered

#### Scenario: Last.fm row with no match inserts self-healing record
- **WHEN** a Last.fm row has no matching track and a non-empty album field
- **THEN** the album is looked up or inserted using normalized matching and a new track with `is_favorite = 1` is inserted linked to that album's `album_id`

#### Scenario: Last.fm row with missing album maps to shared fallback
- **WHEN** a Last.fm row has no matching track and its album field is missing or empty
- **THEN** the track maps to the shared "Unknown Album" record under that artist and is inserted with `is_favorite = 1`

#### Scenario: Last.fm ingestion does not log play history
- **WHEN** Last.fm history is ingested
- **THEN** no time-series entries, historical timestamps, or play count increments are created

### Requirement: Apple Music exported likes CSV contract
For files whose header row begins `Name,Artist,Composer,Album`, ingestion MUST map `record[0]` (`Name`) to `tracks.title`, `record[1]` (`Artist`) to `tracks.artist`/`albums.artist` (falling back to "Unknown Artist" when blank), and `record[3]` (`Album`) to `tracks.album`/`albums.title` (falling back to "Unknown Album" when blank). Rows MUST be reconciled using the text normalization invariants: on a match, `is_favorite` is set to 1 on the existing track; on no match, the album is resolved or inserted and a new track linked to the resolved `album_id` is inserted with `is_favorite = 1`. Additional data attributes (composers, bit rates, time/duration, play counts) MUST be skipped.

#### Scenario: Likes CSV row matches existing track
- **WHEN** a likes CSV row resolves to an existing track
- **THEN** `is_favorite` is set to 1

#### Scenario: Likes CSV row with no match inserts new track
- **WHEN** a likes CSV row has no matching track and a populated album field
- **THEN** the album row is looked up or inserted and a new track linked to the resolved `album_id` is inserted with `is_favorite = 1`

#### Scenario: Likes CSV row with blank album uses shared fallback
- **WHEN** a likes CSV row has no matching track and the album value is missing or empty
- **THEN** the track resolves to the shared "Unknown Album" record under the normalized artist and is inserted with `is_favorite = 1`

#### Scenario: Likes CSV metadata fields skipped
- **WHEN** a likes CSV row is ingested
- **THEN** composers, bit rates, time/duration, and play counts are not written

### Requirement: Apple Music native library sync contract
For the `Apple Music Library Tracks.json` flat JSON array (entries with `Title`, `Track Identifier`, `Artist`, `Album`), ingestion MUST reconcile and parse track records using text normalization. On the positive vector (`Favorite Status - Track` is `true` OR `Track Like Rating` is `"liked"`), ingestion MUST set `is_favorite = 1` and `is_disliked = 0`. On the negative vector (`Track Like Rating` is `"disliked"`), ingestion MUST set `is_disliked = 1` and `is_favorite = 0`.

#### Scenario: Positive vector marks favorite
- **WHEN** a library entry has favorite status `true` or a like rating of `"liked"`
- **THEN** `is_favorite` is set to 1 and `is_disliked` is set to 0

#### Scenario: Negative vector marks dislike
- **WHEN** a library entry has a like rating of `"disliked"`
- **THEN** `is_disliked` is set to 1 and `is_favorite` is set to 0

### Requirement: Play telemetry stream contract
For the `Apple Music Play Activity.csv` wide CSV (headers `Event Timestamp`, `Track Description`, `Play Duration Milliseconds`, `End Reason Type`), rows MUST be processed within an atomic database transaction (`db.BeginTx`) using pre-compiled SQL prepared statement loops. Each fact record MUST track `end_reason_type`, and a boolean flag `was_skipped = 1` MUST be computed and stored when `End Reason Type` contains the substring token `SKIP`.

#### Scenario: Skip detected from end reason
- **WHEN** an `End Reason Type` value contains the substring token `SKIP`
- **THEN** the fact record stores `was_skipped = 1`

#### Scenario: Telemetry rows processed atomically
- **WHEN** a play activity CSV is ingested
- **THEN** rows are processed within a single atomic database transaction using prepared statements

### Requirement: RateYourMusic export contract
For files matching headers `RYM Album ID` and `Rating`, ingestion MUST map `Title` to `albums.title`, the concatenation of `First Name` and `Last Name` to `albums.artist`, `Release_Date` to `albums.release_date`, and `Rating` to `albums.user_rating` divided by 2.0 (scaled to a 5.0 maximum). Ingestion MUST perform an idempotent UPSERT strictly on the `albums` table; on an `album_id` clash it MUST update the existing album's `user_rating` and `release_date`, and track-level properties MUST remain entirely decoupled from this scoring data contract.

#### Scenario: RYM rating scaled to five-point scale
- **WHEN** an RYM rating is ingested
- **THEN** `albums.user_rating` is set to the rating divided by 2.0

#### Scenario: RYM upsert updates existing album only
- **WHEN** an RYM row clashes with an existing `album_id`
- **THEN** the existing album's `user_rating` and `release_date` are updated and no track-level properties are changed

### Requirement: Ambient noise interceptor rule
The streaming telemetry engine MUST filter incoming entries on scan: if the `Container Name`, `Track Description`, or `Artist` values contain any of the case-insensitive substrings `White Noise`, `Bedtime Mix`, `Rain Sounds`, `Sleeping`, `Ocean Waves`, or `Radiance`, the row MUST be skipped entirely and omitted from the database write pass.

#### Scenario: Noise entry filtered from write pass
- **WHEN** a telemetry row's `Container Name`, `Track Description`, or `Artist` contains "Rain Sounds" (case-insensitive)
- **THEN** the row is skipped and not written to the database

#### Scenario: Clean telemetry row retained
- **WHEN** a telemetry row contains none of the noise signatures
- **THEN** the row is written normally

### Requirement: Outlier density compression via track count
During ingestion parsing, the engine MUST ensure the `track_count` field for an album reflects its total distinct track units. The actual score-capping logic MUST be completely decoupled from the ingestion pipeline and handled dynamically within analytical database views using the `track_count` metric.

#### Scenario: Album track count recorded at ingestion
- **WHEN** an album is ingested
- **THEN** `track_count` reflects the album's total distinct track units

#### Scenario: Capping deferred to analytical views
- **WHEN** a high-density album (over 35 individual tracks) is ingested
- **THEN** ingestion stores the full `track_count` and applies no score cap itself

### Requirement: Text normalization invariants
All ingestion routines MUST use standard unicode normalization before executing lookups or inserting records into `clean_title` and `clean_artist`: convert to lowercase and trim spacing, decompose characters canonically (`norm.NFD`), strip characters in the `unicode.Mn` (Mark, nonspacing) category, and recompose using `norm.NFC`.

#### Scenario: Diacritics stripped for matching
- **WHEN** a source string containing combining diacritical marks is normalized
- **THEN** lookups and inserts use the lowercase, diacritical-stripped variant

