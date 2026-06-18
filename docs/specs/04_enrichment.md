# Specification 04: Metadata Enrichment Engine (V1)

## 1. Schema Expansion
- Add a `genres` column (TEXT): Stored as a JSON-serialized string array of deduplicated subgenres on both the `albums` and `tracks` tables.
- Add a `track_count` column (INTEGER): Stored on the `albums` table to lock down absolute structural album volume.

## 2. Enrichment Worker Lifecycle

### A. Target Identification
The background enrichment worker scans the database for records missing structural metadata using a two-tier queue:
1. **Album Queue:** Identifies distinct albums where `albums.genres IS NULL` OR `albums.track_count IS NULL`.
2. **Track Queue:** Identifies individual tracks where `tracks.genres IS NULL`.

### B. Rate-Limiting & Execution
- Enforces strict outbound HTTP request rate-limiting to prevent IP throttling, token depletion, or upstream credential bans.
- Executes fuzzy string matching against external metadata API endpoints using the clean string variants (`clean_artist`, `clean_title`) to ensure high cache hit rates and resilient matching.

### C. Payload Extraction & Commit Invariants
- **For Albums:** Extracts the top 3–5 high-confidence subgenre strings and the absolute physical track count, issuing an `UPDATE` transaction to the target row in the `albums` table.
- **For Tracks:** Extracts or inherits the verified high-confidence subgenre array to mirror parent/artist metadata, committing an `UPDATE` transaction directly to the target row in the `tracks` table.