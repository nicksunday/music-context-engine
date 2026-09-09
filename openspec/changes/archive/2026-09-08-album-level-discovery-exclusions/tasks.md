## 1. Database Exclusion Semantics

- [x] 1.1 Add a typed album-level discovery exclusion representation keyed by normalized artist+album pairs.
- [x] 1.2 Add a database helper that returns albums with `user_rating IS NOT NULL`, albums with durable recommendation-feedback verdicts (`disliked`, `ok`, `good`, `great`, `already_know`), and albums with `not_for_me_today` feedback from the current local calendar day.
- [x] 1.3 Ensure the new helper does not include standalone artist tokens, standalone album-title tokens, track titles, or albums represented only by `tracks` rows.
- [x] 1.4 Update database tests to prove numeric-rated albums, durable feedback albums, and same-day `not_for_me_today` albums are excluded while known artists, track-history-only albums, and older `not_for_me_today` albums remain eligible.

## 2. MCP Verified Discovery Filtering

- [x] 2.1 Route `get_verified_discovery_candidates` through the album-level exclusion helper instead of the flat string exclusion map.
- [x] 2.2 Update candidate filtering to compare normalized artist+album pairs, including available alternate album titles, against album exclusions.
- [x] 2.3 Preserve duplicate-album suppression and the existing per-normalized-artist candidate cap.
- [x] 2.4 Update MCP discovery tests for numeric-rated album exclusion, durable feedback exclusion, same-day `not_for_me_today` exclusion, older `not_for_me_today` eligibility, known-artist eligibility, and track-history-only eligibility.

## 3. Web Recommendation Filtering

- [x] 3.1 Update `/api/recommendations` post-model filtering to use the same album-level exclusion semantics.
- [x] 3.2 Ensure the web endpoint no longer removes candidates solely because their artist or matched starter track appears in local history.
- [x] 3.3 Keep exact albums with numeric ratings, durable feedback verdicts, or same-day `not_for_me_today` cooldowns from being persisted or displayed in generated batches.
- [x] 3.4 Add web endpoint tests covering known-artist unrated albums, numeric-rated album blocking, durable feedback blocking, same-day `not_for_me_today` blocking, and older `not_for_me_today` eligibility.

## 4. Spec And Documentation Alignment

- [x] 4.1 Update recommendation-discovery wording that currently describes artist, track, or flat-token exclusion behavior.
- [x] 4.2 Reconcile this delta with the active `migrate-docs-specs` recommendation-discovery spec so the final archived capability has one album-level exclusion contract.
- [x] 4.3 Keep feedback-memory wording album-scoped: durable feedback excludes the exact album, `not_for_me_today` only excludes the exact album on the same local calendar day, and no feedback excludes the artist or starter track globally.

## 5. Validation

- [x] 5.1 Run `go test ./...`.
- [x] 5.2 Run `openspec validate album-level-discovery-exclusions --strict`.
- [x] 5.3 Manually inspect or query a recommendation fixture where a known artist has an unrated album to confirm it remains eligible.
