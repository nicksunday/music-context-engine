# Specification 02: Ingestion Pipeline Protocol

## 1. Dynamic Source Detection (Content Sniffing)
Instead of matching file name suffixes or paths, the ingestion engine must determine the processing routine dynamically by inspect-reading the first record (header row) of any incoming file argument.

### Header Signature Matrix:
- **Last.fm Layout:** Contains `uts`, `track_mbid`, and `artist`.
- **YouTube Music Uploads Layout:** Contains `Song Title` and `Artist Name 1`.
- **RateYourMusic Layout:** Contains `RYM Album ID` and `Rating`.
- **Apple Music Likes CSV Layout:** Contains `Name,Artist,Composer,Album` (matching the first four header fields).
- **Apple Music Library Track JSON Array:** Detects a top-level JSON array where root objects contain `Track Identifier`, `Title`, and `Content Type`.
- **Apple Music Play Activity Telemetry:** Contains `Event Timestamp`, `Track Description`, and `End Reason Type`.

---

## 2. Source Data Contracts

### A. YouTube Music (Streaming History)
- **Detection Profile:** CSV arrays mapping directly to 3 indices without matching standard headers.
- **Mapping Specifications:**
  - `record[1]` -> `title`
  - `record[2]` -> `album`
  - `record[3]` -> `artist`
- **Affinity Rule:** Execute an idempotent update setting `is_favorite = 1` on the resolved track record.

### B. YouTube Music Uploads
- **Detection Profile:** Matches headers `Song Title` and `Artist Name 1`.
- **Mapping Specifications:**
  - `Song Title` -> `title`
  - `Album Title` -> `album`
  - `Artist Name 1` -> `artist` (Fall back to "Unknown Artist" if empty)

### C. Last.fm History (Historical Validation & Fallback Ingestion)
- **Detection Profile:** Matches headers `uts`, `track_mbid`, and `artist`.
- **Execution Invariant:**
  - Reconcile rows against existing `tracks` and `albums` data using the *Text Normalization Invariants* (Section 5).
  - **Match Found:** Execute an idempotent update setting `is_favorite = 1` on that track record. Do not update or alter existing album relationships.
  - **No Match Found (Data Self-Healing Path):** To resolve missing historical entries without breaking structural database integrity, unmatched rows MUST be inserted as new records rather than skipped:
    1. If the Last.fm record contains a non-empty `album` string, attempt to look up or insert a baseline album record into the `albums` table using normalized matching.
    2. If the Last.fm record's album field is missing or empty, map the track to a generic shared fallback record titled `"Unknown Album"` under that artist.
    3. Insert the new track with `is_favorite = 1`, linking it directly to the corresponding `album_id` resolved in the previous steps.
  - Do not create time-series log entries, record historical timestamps, or increment play count integers.

### D. Apple Music Integration (Exported Likes CSV)
- **Detection Profile:** Matches headers `Name,Artist,Composer,Album` at the start of the file structure.
- **Mapping Specifications:**
  - `record[0]` (`Name`) -> `tracks.title`
  - `record[1]` (`Artist`) -> `tracks.artist` / `albums.artist` (Fall back to "Unknown Artist" if blank)
  - `record[3]` (`Album`) -> `tracks.album` / `albums.title` (Fall back to "Unknown Album" if blank)
- **Execution Invariant:**
  - Reconcile rows against existing data using the *Text Normalization Invariants* (Section 5).
  - **Match Found:** Execute an idempotent update setting `is_favorite = 1` on the existing track record.
  - **No Match Found (Self-Healing Path):** 1. Evaluate the `Album` value. If it is missing or empty, resolve to a shared `"Unknown Album"` record under the normalized artist. If it is populated, lookup or insert the album row into the `albums` table.
    2. Insert the track as a new record in the `tracks` table, linking it directly to the resolved `album_id`.
    3. Ensure the newly inserted row defaults to `is_favorite = 1`.
  - Additional data attributes (Composers, Bit Rates, Time/Duration, Play Counts) are skipped for this metadata context loop; ingestion focuses purely on identity alignment and library compilation.

### E. Apple Music Native Library Sync (`Apple Music Library Tracks.json`)
- **Detection Profile:** Flat JSON array containing entries with fields `Title`, `Track Identifier`, `Artist`, and `Album`.
- **Affinity and Suppression Invariant:**
  - Reconcile and parse track records using text normalization.
  - **Positive Vector:** If `Favorite Status - Track` is equal to `true` OR `Track Like Rating` evaluates to `"liked"`, execute an idempotent update setting `is_favorite = 1` and `is_disliked = 0`.
  - **Negative Vector:** If `Track Like Rating` evaluates to `"disliked"`, execute an idempotent update setting `is_disliked = 1` and `is_favorite = 0` to build our algorithmic aversion ledger.

### F. Play Telemetry Stream (`Apple Music Play Activity.csv`)
- **Detection Profile:** Wide CSV matching headers `Event Timestamp`, `Track Description`, `Play Duration Milliseconds`, and `End Reason Type`.
- **Mapping Specifications:**
  - Process rows within an atomic database transaction (`db.BeginTx`) using pre-compiled SQL prepared statement loops for high performance.
  - Populate a fact record tracking `end_reason_type`. Automatically compute and store a boolean flag `was_skipped = 1` if `End Reason Type` contains the substring token `SKIP`.

---

## 3. RateYourMusic (RYM) Export Data Contracts (Album-Level Valuation)
- **Detection Profile:** Matches headers `RYM Album ID` and `Rating`.
- **Mapping Specifications:**
  - `Title` -> `albums.title`
  - Concatenation of `First Name` + `Last Name` -> `albums.artist`
  - `Release_Date` -> `albums.release_date`
  - `Rating` -> `albums.user_rating` (Explicit Conversion: Divide by 2.0 to scale to 5.0 maximum)
- **Execution Invariant:**
  - Ingestion must perform an idempotent UPSERT strictly on the `albums` table. If an `album_id` clash occurs, update the existing album's `user_rating` and `release_date`. Track-level properties are entirely decoupled from this scoring data contract.

---

## 4. Telemetry Filtering and Density Normalization Invariants

### A. Ambient Noise Interceptor Rule
To prevent therapy blocks, sleeping loops, or rain background tracks from corrupting active profile scoring charts, the streaming telemetry engine must filter incoming entries on scan.
- **Trigger Condition:** If the `Container Name`, `Track Description`, or `Artist` values contain any of the following case-insensitive substring signatures, skip the row entirely and omit it from the database write pass:
  - `White Noise`
  - `Bedtime Mix`
  - `Rain Sounds`
  - `Sleeping`
  - `Ocean Waves`
  - `Radiance`

#### B. Outlier Density Compression (The Score Ceiling Rule)
To prevent massive video game soundtracks, classical box sets, or massive anthologies (assets containing over 35 individual tracks) from scaling profile metrics disproportionately and distorting affinity distributions, the engine enforces a score cap.
- **Rule Threshold:** During ingestion parsing, the engine ensures that the `track_count` field for an album reflects its total distinct track units. 
- **Downstream Scoring Modification:** The actual capping logic is completely decoupled from the ingestion pipeline and handled dynamically within analytical database views using the `track_count` metric.

---

## 5. Text Normalization Invariants
To ensure accent-insensitive searching, reliable cross-platform matching, and true deduplication, all ingestion routines must utilize standard unicode normalization (`norm.NFD`) to strip combining diacritical marks before executing lookups or inserting records into `clean_title` and `clean_artist`.

- **Transformation Logic (Go):**
  1. Convert string to lowercase and trim spacing.
  2. Decompose characters into canonical Unicode decomposition (`norm.NFD`).
  3. Strip out characters belonging to the `unicode.Mn` (Mark, nonspacing) category.
  4. Recompose using `norm.NFC`.