## 1. Limit Contract and Regression Tests

- [x] 1.1 Add focused web tests proving Individual Song requests with limits between 11 and the supported song maximum are not capped at 10.
- [x] 1.2 Add focused web tests proving Individual Song requests above the supported maximum are capped and non-positive limits retain the existing default.
- [x] 1.3 Add focused web tests proving Album requests remain capped at 10 and do not inherit the Individual Song maximum.
- [x] 1.4 Add tests covering the effective limit used for discovery candidate-fetch sizing for both modes, including the larger song limit.

## 2. Mode-Aware Limit Implementation

- [x] 2.1 Search all request-limit, candidate-fetch, ranking, final-trimming, verification, destination-resolution, persistence, and presentation call sites and document which value each stage must consume.
- [x] 2.2 Replace the shared web batch clamp with explicitly named Album and Individual Song maximums while preserving the existing default batch limit.
- [x] 2.3 Apply mode-aware clamping after request mode normalization and propagate the effective limit through recommendation generation and final candidate trimming.
- [x] 2.4 Update discovery candidate-fetch expansion to derive a bounded fetch limit from the effective mode-specific limit without changing the MCP tool maximum.
- [x] 2.5 Verify larger song batches preserve existing graceful degradation, verification, provider timeout, rate-limit, and model-context safeguards; lower the documented song maximum if validation proves 20 unsafe.

## 3. External Workload and Compatibility Coverage

- [x] 3.1 Add or update tests proving partial discovery/provider availability returns only verified available song candidates without fabricated entries or whole-batch failure.
- [x] 3.2 Add or update tests proving existing album generation, exclusion/diversity behavior, persistence, and response fields remain compatible after the limit split.
- [x] 3.3 Confirm the existing MCP `limit` contract and maximum of 50 remain unchanged and are not coupled to web mode-specific limits.

## 4. Documentation and Validation

- [x] 4.1 Update `docs/specs/09_recommendation_discovery.md` with the mode-specific web batch-limit contract and the final Individual Song maximum.
- [x] 4.2 Reconcile the active `migrate-docs-specs` recommendation-discovery delta with the mode-specific limit behavior and document whether the Album limit is attributable to performance, external/API rate limits, verification/link cost, model context, or UI constraints.
- [x] 4.3 Run `gofmt` on changed Go files and focused web/recommendation tests.
- [x] 4.4 Run `go test ./...`.
- [x] 4.5 Run `openspec validate increase-individual-song-batch-limit --strict`.