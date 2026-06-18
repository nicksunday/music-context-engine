# Specification 05: Data Hygiene & Compilation

## 1. Album Deduplication Protocol
The database contains duplicate rows within the `albums` table sharing identical `(clean_title, clean_artist)` keys due to multi-platform ingestion paths. The system must introduce an atomic optimization pass to merge them.

### Algorithmic Execution Rules:
1. **Grouping:** Scan the `albums` table and isolate groupings where `clean_title` and `clean_artist` are identical across multiple primary keys (`id`).
2. **Survivor Election:** Within each duplicate cluster, select exactly one target `id` to survive based on the following metadata priority:
   - **Priority 1:** Keep the row containing an active `user_rating` value (preserving RYM scores).
   - **Priority 2:** Keep the row containing a non-empty, populated `genres` JSON array.
   - **Priority 3:** Fall back to the oldest record (`MIN(id)` or earliest creation state) if metadata conditions are equal.
3. **Foreign Key Realignment:** Update all records in the `tracks` table whose `album_id` points to any of the deprecated duplicate IDs, rewriting them to point strictly to the elected survivor `id`.
4. **Purge:** Execute a hard delete to remove the deprecated, now-orphaned duplicate rows from the `albums` table.

## 2. CLI Integration & Recompilation
- **Command Entry:** Expose this routine via a new CLI subcommand: `go run ./cmd/music-vault optimize`.
- **Feedback Loop:** Upon execution, the routine must print out the total number of redundant album rows successfully purged to `stdout`.
- **Compilation Check:** The final pipeline execution step must force an application rebuild to guarantee the binary natively includes the recently refactored `internal/mcp/server.go` layout alongside the new optimizer package.
