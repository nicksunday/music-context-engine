## Why

External discovery currently fills similar-artist and recording quotas sequentially: one favorite can supply every similar artist, and one prolific artist can exhaust the 48-recording pool before broader genre discovery runs. Recommendations can therefore miss new musical neighborhoods even though MusicBrainz searches extend beyond the local library.

## What Changes

- Select similar artists fairly across the bounded set of affinity seeds, deduplicating and retaining existing exclusions.
- Always attempt prompt-derived tag discovery when tags are available, reserving pool capacity for its results alongside artist searches.
- Merge recording sources fairly before truncation and album genre reconciliation, redistributing unused capacity when sources are empty or fail.
- Preserve verified metadata, exact album exclusions, prompt-fit ranking, artist diversity limits, and bounded external work in both album and song modes.
- Add deterministic regressions for first-seed dominance, prolific catalogs, duplicate results, and partial provider failure.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `recommendation-discovery`: Require fair similar-artist selection and bounded, balanced collection of external artist and prompt-tag candidates before ranking.

## Impact

Changes target `internal/mcp/similarity.go`, `internal/mcp/discovery.go`, their tests, and web recommendation integration tests. Existing Last.fm and MusicBrainz providers remain in use; no new dependency, database migration, public API change, or UI control is required. More of the already bounded source requests may execute, increasing latency compared with the current accidental early exit.
