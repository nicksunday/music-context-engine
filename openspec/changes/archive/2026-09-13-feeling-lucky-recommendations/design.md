## Context

The existing local web UI already has separate song and album recommendation routes, mode-scoped sessions, verified discovery candidates, normalized genre metadata, and destination-link resolvers. The new mode crosses the web, recommendation, persistence, and discovery layers, so its randomness and eligibility rules need to be explicit and testable. See `proposal.md` for motivation and the change specs for the observable contract.

## Goals / Non-Goals

**Goals:**

- Add a third mode without allowing its selection logic to inherit prompt-driven taste ranking.
- Make the random process uniform over distinct observed and admitted profile-inferred genre buckets, then select verified items without personal-preference weights.
- Support a mixed result set of local previously heard items and verified external discoveries, with existing link resolution where applicable.
- Preserve independent Feeling Lucky session restoration and compatibility of the existing song and album modes.
- Make randomness testable through an injectable source of randomness or equivalent deterministic test seam, without exposing random seeds to users.

**Non-Goals:**

- No changes to the meaning of album ratings, song feedback, or durable recommendation exclusions.
- Genre inference is allowed only as a profile-derived expansion of the observed history scope; inferred areas must be labeled, admitted by an explicit threshold/policy, and must not receive extra weight from inference confidence.
- No LLM prompt interpretation, taste-based reranking, or recommendation explanation that claims a personal match.
- No streaming-service authorization, playlist creation, or requirement that every candidate be new to the user.

## Decisions

### D1. Introduce a distinct Feeling Lucky mode contract

Represent Feeling Lucky explicitly in the recommendation type carried through route handling, generation, persistence, latest-batch reads, session history, and session loading. Reuse stable candidate identity and destination fields, while adding the requested count, returned count, sampled genre list, and limited-result state to the mode-specific batch metadata.

*Alternative considered:* encode Feeling Lucky as a special prompt sent through the album or song recommender. Rejected because prompt-driven ranking and profile signals would make the no-weighting guarantee difficult to enforce and test.

### D2. Build the pool from deduplicated observed and inferred genre buckets

Read normalized genre values from listening-history records, normalize casing/whitespace consistently, and deduplicate them into observed genre buckets. Use the existing listening profile to derive additional genre or genre-adjacent buckets, admit only those meeting an explicit inference policy, label them as inferred, and combine them with observed buckets. For each draw, choose an admitted bucket uniformly from the combined set, retrieve verified local and external candidates for that bucket, and choose an item uniformly from the remaining eligible candidates. Deduplicate candidates by normalized item identity across buckets and stop at `N` items or pool exhaustion.

The implementation must not allocate draws by track count, play count, album count, candidate count, inference confidence, ratings, favorites, affinity, recency, or feedback. If an item belongs to multiple selected buckets, it is still emitted once and the remaining draw is filled from another eligible item when possible.

*Alternative considered:* choose uniformly from one flattened candidate pool. Rejected because large genres would dominate the result, contrary to the requested all-genre behavior.

### D3. Separate eligibility from legacy exclusion policy

Add a Feeling Lucky-specific eligibility path. It may include local history/library items and external catalog items; prior listening and prior recommendation appearance are not exclusion reasons. Continue to reject unverifiable records, malformed identities, and candidates that cannot be presented as playable items under the existing destination/identity contract. Do not weaken prompt-driven discovery exclusions for existing callers.

*Alternative considered:* remove all exclusions globally. Rejected because it would change the safety and feedback semantics of song and album recommendations.

### D4. Prefer a local-first, externally topped-up candidate source

For each observed or admitted inferred genre area, gather normalized local items first and query the existing verified external discovery source to top up the pool with items absent from local history. Both sources pass through the same verification, identity normalization, and deduplication boundary before random selection. External failures are treated as source depletion, allowing a batch from valid local candidates instead of failing the entire request.

*Alternative considered:* use only external discovery. Rejected because it would make previously listened-to eligibility unreliable and make the feature dependent on network availability.

### D5. Use explicit mode-aware web pages and session persistence

Add Feeling Lucky to the shared recommendation landing/navigation and give it a dedicated route/container. The page owns the count control and generate/regenerate action, displays returned-versus-requested counts and sampled genres, and uses the existing direct link presentation primitives. Latest-batch and session-list/load reads are filtered by the explicit mode, and navigation never auto-generates.

*Alternative considered:* show Feeling Lucky as a tab inside the album page. Rejected because it would blur session boundaries and make the absence of taste-weighted behavior less clear.

### D6. Validate the count at the boundary

Define a server-side positive maximum for `N`, validate it before candidate retrieval, and mirror the same bounds in the UI control. Return a structured validation error without persistence for invalid counts. For a valid request with too few candidates, persist the partial batch with an explicit limited flag; for no genres, return a non-error setup/enrichment state rather than an empty fabricated batch.

## Risks / Trade-offs

- [Random results can feel repetitive across consecutive requests] → Remove duplicates within each batch, use a fresh randomness source per generation, and permit both local and external candidates; do not add taste weighting to force variety.
- [External discovery can be slow or unavailable] → Use bounded requests, tolerate per-genre source failures, and return valid local candidates with a limited-result explanation.
- [Genre metadata is inconsistent or inferred areas are overly broad] → Reuse existing normalization/enrichment rules, apply an explicit inference admission policy, label observed versus inferred buckets, deduplicate normalized labels, and expose sampled labels so the source scope is inspectable.
- [Users may interpret “unbiased” as “all genres equally represented in every small batch”] → Record the sampled genre set and explain that selection is uniform across eligible genre buckets while the finite batch may not contain every genre.
- [A shared candidate can appear under multiple genres] → Deduplicate on normalized item identity before final persistence and continue draws until the requested count or eligible pool is exhausted.
- [New mode fields could break legacy session records] → Treat absent mode metadata as existing album-mode data, use additive persistence changes, and keep existing response fields stable.

## Migration Plan

1. Add the explicit Feeling Lucky recommendation type and additive batch/session metadata with legacy records defaulting to album mode.
2. Implement the isolated historical-genre candidate builder and unbiased selector behind the new mode contract, including deterministic test injection and source-failure handling.
3. Add mode-aware generation/read endpoints and the dedicated web route, then add shared navigation and the count/result states.
4. Backfill no data; existing rows remain unchanged and Feeling Lucky becomes available immediately for profiles with normalized genres.
5. Verify focused selector, API, persistence, and UI tests, then run the full Go test suite and strict OpenSpec validation.
6. Roll back by disabling the Feeling Lucky route and generation type; additive batch/session rows can remain inert and existing song/album records require no destructive rollback.