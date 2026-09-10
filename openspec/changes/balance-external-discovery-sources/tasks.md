## 1. Balance similar-artist selection

- [x] 1.1 Inspect shared similarity-helper callers and retain bounded seed counts; normalize and deduplicate input seeds before provider lookup.
- [x] 1.2 Collect bounded per-seed neighbor lists and select eligible unique names in rounds while preserving exclusions and best-effort failures.
- [x] 1.3 Add deterministic similarity tests for four productive seeds, duplicate/excluded neighbors, sparse and failed sources, missing configuration, and lookup/output bounds.

## 2. Balance recording sources

- [x] 2.1 Refactor seeded recording retrieval to attempt tags first when present and collect each bounded artist source separately without stopping on the first full catalog.
- [x] 2.2 Implement the 24-of-48 tag reservation, fair artist rounds, recording deduplication, and redistribution of unused capacity while retaining tag-only behavior.
- [x] 2.3 Add deterministic collection tests for a dominant first artist, 24 tag records plus six records from each of four artists, cross-source overlaps, sparse results, tag-only and artist-only requests, and partial failures.
- [x] 2.4 Verify cancellation, per-source request limits, rate limiting, and the 48-record cap with fake HTTP providers; ensure no pagination or retry expansion.

## 3. Verify recommendation integration

- [x] 3.1 Cover album and song paths with fake-provider regressions proving unfamiliar tag-discovered artists remain eligible and both modes use balanced collection; retain album-only genre reconciliation.
- [x] 3.2 Verify exact album exclusions, duplicate-album suppression, artist payload caps, and prompt-fit ranking still apply after source balancing, including when filtering yields a smaller batch.
- [x] 3.3 Run `go test ./internal/mcp ./internal/web` and `go test ./...`; record results and any unresolved limitations before marking implementation complete.

## Validation

- `go test ./internal/mcp ./internal/web` passed.
- `go test ./...` passed.
- `git diff --check` passed.
- New fake-provider tests verify fair seed selection, tag reservation, source failures, deduplication, request counts, rate limiting, cancellation, and album/song filtering. Existing web prompt-fit and feedback-ranking regressions also passed.
- Test servers required execution outside the filesystem sandbox because sandboxed port binding was denied.
- Remaining design limitation: filtering can shrink the balanced raw pool; source quotas do not override eligibility or final prompt-fit ranking. Live provider latency was not benchmarked.
