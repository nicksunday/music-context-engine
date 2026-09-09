## 1. Discovery pool and bounded variety

- [x] 1.1 Add request-scoped randomness injection and a bounded tie/jitter policy to MusicBrainz candidate ordering without changing exclusion or fetch caps.
- [x] 1.2 Preserve prompt-fit precedence while distributing equivalent candidates across normalized artists, albums, and tracks.
- [x] 1.3 Add MCP discovery tests for duplicate suppression, artist caps, varied equivalent results, and exact effective limits.

## 2. Batch selection and backfill

- [x] 2.1 Refactor model-selected, fallback, and final web batch assembly to walk the complete verified pool and backfill after diversity filtering.
- [x] 2.2 Apply mode-specific identity rules: one album per artist by default for albums, unique track identities plus artist/album variety for songs, and explicit deep-dive exceptions.
- [x] 2.3 Add web tests proving under-selection and diversity removal still reach the requested count, while genuine pool shortage returns only eligible candidates.

## 3. Feedback-aware fit

- [x] 3.1 Build a lightweight recent-feedback feature summary from existing persisted verdicts, resolving genre tags from the linked recommendation candidate with local album metadata as a fallback, plus recency decay and normalized artist/album/tag matching.
- [x] 3.2 Incorporate the feedback score as a bounded ranking adjustment that cannot override exclusions, avoid filters, or clear prompt-fit signals.
- [x] 3.3 Add tests for positive preference, negative repetition penalty, stale/absent feedback, and explicit prompt override.

## 4. Diagnostics and validation

- [x] 4.1 Add internal structured shortfall diagnostics for candidate exhaustion and post-filter removal without exposing private prompt contents.
- [x] 4.2 Run `gofmt` and the complete Go test suite, then verify the changed recommendation behavior through endpoint-level tests.
- [x] 4.3 Update recommendation documentation to explain bounded randomness, backfill behavior, and why reinforcement learning is deferred.