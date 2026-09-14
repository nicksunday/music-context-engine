## 1. Recommendation Type and Persistence

- [x] 1.1 Inventory the existing recommendation mode enum/contracts, web routes, generation endpoints, batch/session tables, and latest/session-load queries.
- [x] 1.2 Add the Feeling Lucky recommendation type with backward-compatible defaults for legacy song and album records.
- [x] 1.3 Add additive batch metadata for requested count, returned count, sampled genres, and limited-result state; preserve candidate identity and destination fields.
- [x] 1.4 Scope latest-batch, session-list, session-load, generation, and response serialization by Feeling Lucky mode without changing song or album semantics.

## 2. Historical Genre Candidate Selection

- [x] 2.1 Implement normalized, deduplicated observed genre extraction from listening-history records and profile-derived inferred/genre-adjacent areas with an explicit admission policy.
- [x] 2.2 Implement a Feeling Lucky candidate source that combines valid local heard items with verified external candidates for observed and admitted inferred genre areas.
- [x] 2.3 Add uniform observed/inferred genre-bucket and candidate selection using an injectable deterministic randomness seam for tests; exclude frequency, inference confidence, taste, ratings, favorites, feedback, recency, affinity, and quality weights.
- [x] 2.4 Preserve observed-versus-inferred genre labels in the batch response and UI so inferred scope is transparent.
- [x] 2.5 Permit previously listened-to and previously recommended items in Feeling Lucky while retaining verification, identity, and playable-item checks.
- [x] 2.6 Deduplicate candidates across genres and sources, stop at the validated `N`, and return explicit partial/empty outcomes without fabricating candidates.
- [x] 2.7 Validate `N` at the API boundary against the supported positive range and ensure invalid requests do not persist batches.

## 3. Web Routes and Presentation

- [x] 3.1 Add Feeling Lucky to the Recommendation Engine entry surface and shared navigation alongside Song Recommendations and Album Recommendations.
- [x] 3.2 Add a dedicated Feeling Lucky route/page/container with mode-specific session loading and no automatic generation on navigation or refresh.
- [x] 3.3 Add the visible count control, Generate/Feeling Lucky action, duplicate-submit protection, and loading/error/empty/setup states.
- [x] 3.4 Render mixed song/album candidates with verified identity, available direct destination links, sampled genre context, and returned-versus-requested counts.
- [x] 3.5 Render limited-result messaging that distinguishes an available partial batch from missing genre enrichment or an upstream failure.

## 4. Verification

- [x] 4.1 Add selector tests proving uniform observed/inferred genre eligibility despite unequal listening/candidate volumes or inference confidence and proving taste signals do not affect output selection.
- [x] 4.2 Add candidate tests for inferred-genre admission/labeling, previously heard items, new external items, cross-source/genre deduplication, invalid candidates, source failures, partial batches, and profiles with neither observed nor inferred genres.
- [x] 4.3 Add API/persistence tests for count validation, mode-isolated latest/session reads, metadata round trips, and legacy song/album compatibility.
- [x] 4.4 Add web contract tests for landing/navigation, direct Feeling Lucky route access, refresh restoration, controls, loading/error/empty/limited states, and no cross-mode batch leakage.
- [x] 4.5 Run formatting, focused recommendation tests, the full Go test suite, and strict OpenSpec validation for `feeling-lucky-recommendations`.