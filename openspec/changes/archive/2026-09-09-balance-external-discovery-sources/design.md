## Context

See proposal.md for motivation. `SimilarArtistNames` currently fills its total quota while iterating each affinity seed; the web caller supplies up to four favorites and asks for four discovery artists. `searchSeededRecordings` then appends artist recordings until the 48-record cap and only searches tags if space remains. Later album deduplication and the two-candidates-per-artist payload cap cannot restore discarded source breadth. Album and song discovery share this collection path; only albums reconcile release-group genres.

## Goals / Non-Goals

**Goals:** Make bounded source collection fair before truncation, guarantee a prompt-tag search opportunity, and keep the merge deterministic and independently testable.

**Non-Goals:** Add providers, change model planning or ranking, require every recommendation to be a new artist, expand artist exclusions to the whole library, introduce pagination, or guarantee full batches after filtering. Explicit prompt-reference seeding and retrieval-time album eligibility optimization are separate work.

## Decisions

### Collect similar-artist lists before selecting names

Normalize and deduplicate the bounded input seeds, query each once using the existing per-seed result limit, then take one eligible unique neighbor per seed per round until the requested maximum is reached. Skip excluded and duplicate names without consuming a turn's successful selection; exhausted or failed lists drop out. Preserve provider order within lists and affinity order between lists. The web flow retains its four-input/four-output bounds; inspect other helper callers to retain their existing bounds.

Randomizing the current sequential fill would merely rotate which favorite dominates. Round-robin selection guarantees contribution opportunities and supports deterministic tests.

### Reserve half the seeded recording pool for tags

When tags exist, run the tag search first so a slow catalog cannot consume the whole request lifetime before prompt discovery begins. Then attempt each distinct bounded artist seed once, retaining separately bounded result lists. Retain current per-search limits, HTTP serialization, identification, cancellation, and rate limiting; add no retry or pagination loop.

Build the merged pool in three stages: admit up to 24 unique tag recordings; fill remaining positions in rounds across artist lists; then use remaining tag results if artist lists exhaust before reaching 48. With no tags, artist lists can fill all 48 positions. With no seeds, preserve the existing tag-only path and its request-derived bounds. This policy reserves meaningful space for prompt intent while letting sparse sources donate unused capacity.

Deduplicate with the existing recording identity key. A record already admitted from tags also appearing in an artist list is skipped without consuming that artist's successful turn; scan onward for its next unique record. No provenance field needs to enter persisted or public candidate schemas. Do not prematurely truncate a source list to its nominal share: overlap may require scanning farther to fill unique positions.

Equal round-robin across tags and four artists would allocate only one fifth to the prompt source. A half-pool reservation gives prompt discovery meaningful representation without changing downstream fit ranking. Simply increasing the cap raises reconciliation costs without addressing starvation.

### Apply source balance before existing candidate processing

Both `SearchWithSeeds` and `SearchSongs` use the balanced recording merge. Keep parsing, album genre reconciliation, album exclusions, candidate diversification, payload caps, avoid filtering, prompt/feedback ranking, and model selection in their existing stages. Source representation is a retrieval guarantee, not a displayed-batch quota. Tests must inspect the merged source pool separately from final filtered candidates.

Preserve best-effort handling of individual seeded source failures and the existing no-candidate behavior. Successful sources backfill failed or empty sources. Cancellation stops further work; it is not a reason to issue additional searches.

## Risks / Trade-offs

- More bounded requests actually execute → retain at most four similar-artist lookups in the web flow, four artist recording searches plus one tag search, existing per-call deadlines, and the 48-record reconciliation cap; verify request counts with fake providers.
- Raw tag matches may be broad or duplicate-heavy → keep metadata validation and prompt-fit ranking authoritative; do not force a final source quota.
- Album deduplication and exclusions can still shrink a balanced pool → test this explicitly and permit smaller valid batches; eligibility-aware refilling is outside this change.
- Provider ordering can still favor popular neighbors within each seed → fairness addresses cross-source starvation, not comprehensive catalog coverage.

## Migration Plan

No data migration or configuration change. Implement the helpers and regression tests, verify both web modes, and run the Go suite. Deploy through the normal build/restart workflow. Rollback is a code revert; previously saved recommendations remain valid.
