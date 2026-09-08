## Purpose

Defines the background metadata enrichment engine that fills in structural metadata — genre arrays and physical track counts — for albums and tracks by querying external metadata APIs under rate limits and committing verified payloads.

## Requirements

### Requirement: Schema expansion for enrichment output
The system SHALL store enrichment results in a `genres` column (TEXT, JSON-serialized array of deduplicated subgenres) on both the `albums` and `tracks` tables, and a `track_count` column (INTEGER) on the `albums` table to lock down absolute structural album volume.

#### Scenario: Enrichment columns present on both tables
- **WHEN** the database schema is inspected
- **THEN** `albums` and `tracks` expose a `genres` column and `albums` exposes a `track_count` column

### Requirement: Enrichment target identification
The background enrichment worker MUST scan the database for records missing structural metadata using a two-tier queue: (1) Album Queue — distinct albums where `albums.genres IS NULL` OR `albums.track_count IS NULL`; (2) Track Queue — individual tracks where `tracks.genres IS NULL`.

#### Scenario: Album with missing metadata queued
- **WHEN** an album has NULL `genres` or NULL `track_count`
- **THEN** it is added to the album enrichment queue

#### Scenario: Track with missing genres queued
- **WHEN** a track has NULL `genres`
- **THEN** it is added to the track enrichment queue

### Requirement: Enrichment rate limiting and matching
The worker MUST enforce strict outbound HTTP request rate-limiting to prevent IP throttling, token depletion, or upstream credential bans, and MUST execute fuzzy string matching against external metadata API endpoints using the clean string variants (`clean_artist`, `clean_title`) to ensure high cache hit rates and resilient matching.

#### Scenario: Outbound requests rate limited
- **WHEN** the worker issues external metadata requests
- **THEN** they respect a strict rate limit

#### Scenario: Lookups use clean string variants
- **WHEN** an album or track is matched against an external metadata API
- **THEN** matching uses `clean_artist` and `clean_title`

### Requirement: Album payload extraction and commit
For albums, the worker MUST extract the top 3–5 high-confidence subgenre strings and the absolute physical track count, issuing an `UPDATE` transaction to the target row in the `albums` table.

#### Scenario: Album enriched with genres and track count
- **WHEN** an album is processed by the worker
- **THEN** its row is updated with 3–5 high-confidence subgenres and its absolute physical track count

### Requirement: Track payload extraction and commit
For tracks, the worker MUST extract or inherit the verified high-confidence subgenre array to mirror parent/artist metadata, committing an `UPDATE` transaction directly to the target row in the `tracks` table.

#### Scenario: Track inherits verified subgenres
- **WHEN** a track is processed by the worker
- **THEN** its `genres` array is populated with the verified high-confidence subgenres mirroring parent/artist metadata

### Requirement: Last.fm tag filtering
Last.fm tags MUST NOT be constrained by a hard-coded genre whitelist. The worker SHALL accept normalized musical descriptors broadly and reject only obvious non-musical/social noise such as `seen live`, favorites/ownership tags, URL/platform tags, and year/decade buckets.

#### Scenario: Non-musical tag rejected
- **WHEN** a Last.fm tag such as "seen live" or a year/decade bucket is returned
- **THEN** it is filtered out of the stored genres array

#### Scenario: Broad musical descriptors accepted
- **WHEN** a normalized musical descriptor outside a hard-coded whitelist is returned
- **THEN** it is accepted into the genres array
