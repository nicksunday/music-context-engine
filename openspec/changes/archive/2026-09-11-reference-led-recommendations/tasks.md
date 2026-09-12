## 1. Baseline and evidence contracts

- [x] 1.1 Capture the exact Symphony X song prompt and a clearly labeled album variant including Children of Bodom; record current model/settings, outputs, and available timings locally before changing behavior.
- [x] 1.2 Define versioned reference, similarity-edge, scoped-evidence, qualification, provider-status, and trace structures with legacy decoding coverage.
- [x] 1.3 Add stage timing and call-count instrumentation around planning, similarity, discovery, verification, selection, persistence, and destination resolution; verify credentials never enter traces.

## 2. Reference-led retrieval

- [x] 2.1 Add optional RecommendationRequest.Examples and modelDiscoveryPlan.References using the approved contract in design decision 1; extract prose references in the existing planning call, reconcile explicit-example precedence, preserve unresolved identities, and remove fixed artist-specific trait bundles. Test plain-text Symphony X, combined references, conflicting model interpretations, ambiguous projects, invalid examples, and unchanged legacy requests without an extra model call.
- [x] 2.2 Return structured Last.fm neighbor edges with scores and all contributing reference paths while preserving normalized deduplication and fair source selection.
- [x] 2.3 Seed discovery from current positive references/examples, using affinity only for requests without positive references; preserve existing tag reservation, collection bounds, exclusions, and diversity.
- [x] 2.4 Expose safe similarity configuration/outcome diagnostics and test missing key, timeout, no results, unresolved projects, and bounded tag fallback.

## 3. Personal context and contextual feedback

- [x] 3.1 Add additive persistence and correction/removal support for the request-scoped artist/album/song examples introduced in task 2.1, including polarity, notes, and immutable generation copies.
- [x] 3.2 Build bounded relevance-based context queries over existing favorites, ratings, listening summaries, taste feedback, and current fit judgments; test sparse history and unrelated-history dominance.
- [x] 3.3 Retrieve pre-planning context from structured examples and conservative exact normalized library-name matches; enrich selection context using reconciled plan references and neighbors within the two-model-call design. Preserve original request, entity scope, and signal type; do not interpret preliminary name matches as positive reference intent.
- [x] 3.4 Add the example editor and retained-request restoration to the web UI; verify artist examples require no track and fit/taste judgments remain independent.
- [x] 3.5 Test that corrected or cleared feedback changes future retrieved context while historical snapshots, global ratings, and unrelated-request eligibility remain intact.

## 4. Catalog assessment and qualified selection

- [x] 4.1 Replace genre-derived performance support flags with scoped evidence and explicit uncertainty; preserve tags as broad search/compatibility signals.
- [x] 4.2 Extend the selection response to assess verified candidate indexes with fit categories and evidence references; validate evidence existence, scope, and contradictions.
- [x] 4.3 Implement the design's supported/plausible qualification policy for album and song entries; test heterogeneous catalogs, missing track evidence, hard requirements, and general tag requests.
- [x] 4.4 Apply qualification consistently to model selections, backfill, diversity replacement, and fallback; support smaller and empty successful batches with clear reasons.
- [x] 4.5 Expose degraded model-selection outcomes in responses and UI; test malformed JSON, timeouts, exhausted pools, and unchanged identity/link/exclusion contracts.

## 5. Inspection and evaluation

- [x] 5.1 Extend immutable generation snapshots with bounded stage evidence and effective examples; add local trace inspection and legacy missing-provenance handling.
- [x] 5.2 Extend offline replay fixtures for reference isolation, duplicate provenance, missing evidence, taste-versus-fit disagreement, qualification, and bounded provider work.
- [x] 5.3 Add an explicitly invoked live/manual evaluation protocol and report format covering user fit, yield, empty outcomes, provider failures, stage latency, total latency, and call counts.
- [x] 5.4 Add frozen-pool model-comparison support and separate fresh-planning evaluation; record model availability limitations without downloading or changing the default model implicitly.
- [x] 5.5 Run before/after evaluation on recurring and held-out requests, report subjective judgments separately from automated checks, and include median/p95 with sample counts and comparable settings.

## 6. Validation and documentation

- [x] 6.1 Run affected Go tests and offline corpus evaluation, plus browser coverage for example editing, fit feedback, shortfall/degraded responses, and retained batches.
- [x] 6.2 Verify additive migration and legacy reads against a disposable database copy, without rewriting personal history or deleting feedback.
- [x] 6.3 Document Last.fm environment setup and configuration status, evidence limitations, request-specific learning, local trace inspection, and evaluation procedures.
- [x] 6.4 Review quality and latency findings against call/deadline bounds; update the deferred caching note with measured priorities without implementing caching or training.
