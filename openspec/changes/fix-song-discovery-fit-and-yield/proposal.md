## Why

Listening tests returned only 11 and 8 songs for requests of 20, repeated artists, and several substantial fit or metadata failures. Even the broad request “Neoclassical metal” can return nothing because a genre keyword activates a performance-evidence gate that normal discovery metadata cannot satisfy.

## What Changes

- Separate genre intent from explicit performance requirements; missing performance evidence is uncertainty, while explicit hard constraints remain enforceable.
- Make discovery capacity and deduplication appropriate for songs, with bounded expansion and artist variety that does not require one song per album.
- Resolve artist identities before attaching similarity or genre evidence; preserve evidence scope and prevent namesake contamination.
- Use request-fit feedback to distinguish musicianship, vocals, and energy. Preserve Obsidious as a positive discovery despite greater heaviness; do not infer a clean-vocals-only constraint or a technical-death-metal blacklist.
- Retain actionable candidate-stage diagnostics, including successful empty outcomes, and add regression cases plus a listening acceptance protocol.
- Keep the removed example editor out of scope; use the existing prompt and fit-feedback surfaces.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `recommendation-discovery`: Genre/performance qualification, identity-grounded retrieval, bounded song pool expansion, and artist variety.
- `song-recommendations`: Recording-level deduplication and scoped metadata for song candidates.
- `recommendation-fit-evaluation`: Regression coverage and traceable shortfalls including empty attempts.

## Impact

Touches discovery/provider contracts in `internal/mcp`, planning/qualification/selection in `internal/web`, additive trace persistence, and evaluation fixtures. Existing album behavior, durable taste feedback, historical snapshots, and the separate Feeling Lucky proposal must remain compatible. No model replacement, training, automatic upload, or example-editor restoration is included.
