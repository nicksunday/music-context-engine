## Why

Requests for technically proficient metal in the vein of Symphony X repeatedly return the wrong musical feel, despite extensive personal listening data. The current pipeline collapses distinct references into broad genre proxies, seeds discovery from historical favorites, and fills batches without establishing sufficient prompt fit.

## What Changes

- Preserve free-form requests and accept optional structured examples; extract prose references in the existing planning call and reconcile them with explicit examples taking precedence.
- Seed bounded, direct Last.fm artist-similarity discovery from current request references; retain reference identity, match score, and source provenance. Fall back explicitly when the provider is unavailable.
- Remove shared artist-specific trait bundles and unsupported genre-to-performance assertions; separate inferred intent from verified musical evidence.
- Select songs and albums within related artists' catalogs using evidence appropriate to the requested entity, preserving uncertainty.
- Retrieve bounded, request-relevant local favorites, ratings, listening summaries, and feedback instead of relying only on global affinity summaries.
- Reuse separate taste and prompt-fit feedback, add request-scoped positive/negative reference examples, and preserve their context for future retrieval.
- Replace unconditional batch filling with fit-qualified selection and clear shortfall/degraded-selection outcomes.
- Add local stage traces, end-to-end quality evaluation, and latency/call-count measurements. Compare backend models on frozen candidate pools after retrieval improvements.
- Record caching as a deferred follow-up; preserve bounded calls, deadlines, and provider rate limits now.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `recommendation-discovery`: Reference-led retrieval, evidence-aware catalog selection, relevant personal context, qualified batch filling, and explicit degraded outcomes.
- `recommendation-prompt-fit-feedback`: Contextual reference examples and reuse of request-specific fit judgments without conflating them with taste.
- `recommendation-fit-evaluation`: Local stage traces, live evaluation protocol, model comparison, and latency measurement alongside deterministic replay.

## Impact

Changes affect Go discovery/similarity clients, web orchestration and prompts, SQLite context/feedback/snapshot persistence, feedback UI/API, and evaluation tooling. Existing album exclusions, song identity/link behavior, and retained batches remain supported. Existing Last.fm and MusicBrainz integrations suffice; no new streaming-account integration is required. Batch counts may decrease when evidence is insufficient, deliberately changing the existing fill-to-limit behavior.

## Non-goals

Model fine-tuning or reinforcement learning, automatic training/upload, audio analysis, a new metadata provider, recursive artist-graph traversal, caching implementation, and replacing the backend model by default. Caching is recorded in `docs/future-work/recommendation-caching.md` for a later proposal informed by measurements.
