## Purpose

Defines the data hygiene and compilation pass that detects duplicate album rows created by multi-platform ingestion, merges them into a single surviving record, realigns track references, and exposes the operation through the CLI.

## Requirements

### Requirement: Album deduplication grouping
The system MUST scan the `albums` table and isolate groupings where `clean_title` and `clean_artist` are identical across multiple primary keys (`id`).

#### Scenario: Duplicate cluster isolated
- **WHEN** two or more album rows share the same `clean_title` and `clean_artist`
- **THEN** they are grouped as a duplicate cluster

### Requirement: Survivor election
Within each duplicate cluster, the system MUST elect exactly one target `id` to survive based on the following metadata priority: **Priority 1** — keep the row containing an active `user_rating` value (preserving RYM scores); **Priority 2** — keep the row containing a non-empty, populated `genres` JSON array; **Priority 3** — fall back to the oldest record (`MIN(id)` or earliest creation state) when metadata conditions are equal.

#### Scenario: Rated album survives
- **WHEN** a duplicate cluster contains a row with an active `user_rating`
- **THEN** that row is elected survivor

#### Scenario: Populated genres tiebreak
- **WHEN** no row in a cluster has a `user_rating` but one row has a populated `genres` array
- **THEN** the row with the populated `genres` array is elected survivor

#### Scenario: Oldest record as final fallback
- **WHEN** a cluster's rows have equal metadata conditions
- **THEN** the oldest record (`MIN(id)` or earliest creation state) is elected survivor

### Requirement: Foreign key realignment
The system MUST update all records in the `tracks` table whose `album_id` points to any deprecated duplicate id, rewriting them to point strictly to the elected survivor `id`.

#### Scenario: Track references rewritten to survivor
- **WHEN** a track's `album_id` references a deprecated duplicate id
- **THEN** the `album_id` is rewritten to the elected survivor `id`

### Requirement: Deprecated album purge
The system MUST execute a hard delete to remove the deprecated, now-orphaned duplicate rows from the `albums` table.

#### Scenario: Orphaned duplicates removed
- **WHEN** the deduplication pass runs
- **THEN** deprecated duplicate album rows are hard-deleted

### Requirement: Optimize CLI integration
The routine MUST be exposed via a new CLI subcommand: `go run ./cmd/music-vault optimize`. Upon execution it MUST print the total number of redundant album rows successfully purged to `stdout`, and the final pipeline execution step MUST force an application rebuild so the binary natively includes the refactored `internal/mcp/server.go` layout alongside the new optimizer package.

#### Scenario: Purge count reported to stdout
- **WHEN** the user runs `go run ./cmd/music-vault optimize`
- **THEN** the total number of purged redundant album rows is printed to `stdout`
