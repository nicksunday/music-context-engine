## Why

Individual Song batches are currently capped at 10, which makes larger song-focused requests feel unnecessarily constrained even though song candidates are already a supported recommendation mode. Album batches should not automatically inherit a larger ceiling: album discovery and verification may produce more expensive external/API work per candidate, and the repository currently applies one shared clamp without documenting that trade-off. This change separates the two policies and establishes evidence for the album limit instead of treating it as an unexplained product restriction.

## What Changes

- Raise the maximum accepted `limit` for Individual Song recommendation requests above the current value of 10; the planning baseline is 20 unless implementation profiling or provider constraints require a lower documented value.
- Keep an explicit, independently configurable maximum for Album recommendation requests rather than coupling it to the song maximum.
- Preserve the existing default batch size and request/response contracts unless a focused compatibility adjustment is required.
- Ensure song-mode discovery retrieval, ranking, verification, persistence, and rendering honor the larger effective limit without silently truncating at 10.
- Document and test the relationship between requested limits, effective limits, candidate-fetch expansion, and bounded external/API work.
- Investigate whether the album ceiling is driven by performance, MusicBrainz/provider rate limits, verification/link lookup cost, model context size, or UI usability; record the conclusion and retain a conservative album cap where those constraints apply.
- Add regression coverage proving album requests remain within their own limit and existing album behavior is unchanged.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `recommendation-discovery`: change the recommendation batch-limit contract so Individual Song and Album modes have explicit, independently enforced maximums, with larger song batches supported and album constraints preserved or clarified.

## Impact

- **`internal/web/server.go`**: mode-aware request clamping and discovery candidate-fetch sizing; likely separation of shared constants/helpers.
- **`internal/web/server_test.go`**: endpoint and helper coverage for song limits above 10, album-limit enforcement, defaults, and downstream fetch bounds.
- **Recommendation/discovery documentation and OpenSpec migration artifacts**: document the mode-specific limit contract and the evidence behind the album ceiling.
- **External services**: larger song requests can increase MusicBrainz and destination-link lookups, so bounded concurrency, timeouts, rate limiting, and model-context limits must remain intact.
- **Persistence/UI**: no schema change is expected; existing batch storage and rendering should support the larger song candidate list without changing album behavior.