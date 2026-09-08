## Context

The web server currently normalizes the request mode and then applies one shared `clampBatchLimit`, with a default of 6 and a maximum of 10. Candidate retrieval derives its fetch size from that clamped value, while song mode may perform additional verification and destination-link resolution. The MCP discovery contract has its own maximum of 50, so the change concerns the web recommendation workflow rather than the lower-level tool contract. See `proposal.md` for motivation and `specs/recommendation-discovery/spec.md` for the required behavior.

## Goals / Non-Goals

**Goals:**

- Introduce explicit mode-aware effective-limit calculation for Album and Individual Song requests.
- Set the initial song maximum to 20, subject to implementation validation of timeouts, API/rate-limit behavior, and model context size.
- Preserve the album maximum at 10 and preserve the existing default of 6 unless evidence requires otherwise.
- Ensure every stage that depends on batch size uses the same effective mode-specific limit.
- Add tests that make the old shared-cap regression and external-work bounds observable.

**Non-Goals:**

- Do not change the MCP tool's independent maximum of 50.
- Do not add user-configurable limits, pagination, background jobs, or playlist behavior.
- Do not alter database schema or change the meaning of persisted album/song candidate fields.
- Do not increase provider concurrency or remove existing timeout/rate-limit safeguards.

## Decisions

### D1. Calculate the limit after mode normalization

Keep mode detection as the source of truth and replace the shared clamp with a mode-aware helper, such as `clampBatchLimit(mode, requested)`. This prevents a song request from being capped by the album policy while ensuring all downstream callers receive one normalized effective value. The helper should retain the existing default behavior and cap non-positive/oversized values deterministically.

**Alternative considered:** raise the single `maxBatchLimit` constant. Rejected because it would silently increase album workloads and would not answer whether album requests have different performance/API constraints.

### D2. Keep album and song maxima separate and named

Use independently named constants for the album maximum and the target song maximum. Keep the album value at 10 initially. During implementation, inspect the full song pipeline and measure or reason about candidate fetch expansion, MusicBrainz requests, verification, link lookups, Ollama context, and UI rendering; document the result in the recommendation-discovery documentation and tests.

**Alternative considered:** make both modes use 20 for consistency. Rejected because albums currently have album-level exclusion, diversity filtering, and album-link verification costs that may not scale like song output.

### D3. Preserve bounded expansion rather than multiplying every downstream operation blindly

Derive candidate retrieval from the effective limit using the existing expansion policy, then apply the existing service safeguards. Verify that ranking and final trimming use the effective limit and that provider resolution does not create an unbounded fan-out. If the larger song batch reveals a provider or model-context ceiling, lower and document the safe song maximum rather than weakening safeguards.

**Alternative considered:** fetch exactly the requested number of candidates. Rejected because current filtering and verification can remove candidates and the existing padded pool is intended to prevent starvation.

### D4. Treat incomplete batches as valid graceful degradation

Do not fabricate candidates or fail an otherwise valid request solely because external discovery or destination providers cannot fill the requested count. Return the verified subset according to the existing error/degradation contract, while retaining the effective limit in the internal flow and response semantics already supported by the API.

## Risks / Trade-offs

- [A larger song batch increases external requests and latency] → retain bounded candidate expansion, existing timeouts/rate limiting, and focused tests for fetch bounds and partial results.
- [Ollama context becomes too large for 20 candidates] → validate prompt/payload size and lower the documented song maximum if necessary; do not bypass context safeguards.
- [The shared clamp is used in an overlooked path] → search all limit/fetch/trimming call sites and add endpoint-level tests for both modes.
- [Album rationale remains uncertain] → record measured/request-level evidence during implementation; keep the independent album cap unchanged until evidence supports a separate change.
- [UI becomes crowded with larger batches] → preserve existing rendering semantics and explicitly treat responsive layout/pagination as out of scope for this change.

## Migration Plan

1. Add mode-specific limit tests and inspect all request-limit consumers.
2. Implement the mode-aware clamp and update candidate-fetch sizing and any downstream trimming paths.
3. Run focused web/recommendation tests and the complete Go test suite.
4. Update the recommendation-discovery documentation and active migration delta with the final song maximum and album-limit rationale.
5. Roll back by restoring the shared maximum of 10 if production/local profiling identifies unacceptable latency or provider pressure; no data migration is required.