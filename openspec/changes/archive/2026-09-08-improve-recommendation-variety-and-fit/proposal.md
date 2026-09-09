## Why

Recommendation batches can be shorter than the requested limit after model selection, verification, and artist/album diversity filtering. Repeated MusicBrainz ordering and deterministic ranking also make repeated prompts—especially comparison prompts such as “same vein as Symphony X”—feel stuck on the same tracks or drift toward broad genre matches instead of the requested musical traits.

## What Changes

- Retrieve and rank a larger verified candidate pool, then backfill the displayed batch after duplicate and diversity filtering.
- Add bounded, reproducible-per-request randomness so eligible tracks and albums vary between refreshes without allowing weak candidates to outrank materially better prompt matches.
- Apply diversity at both the candidate-pool and final-batch stages, with album mode favoring distinct artists/albums and song mode favoring distinct tracks, artists, and albums.
- Incorporate durable feedback as a ranking signal where candidates remain eligible, while preserving existing exact-album exclusions.
- Add diagnostics and tests covering requested-limit fulfillment, repeat suppression, variety, and prompt-fit ranking.
- Do not introduce reinforcement learning in this iteration; accumulate clean feedback signals first so a later bandit or learning-to-rank experiment can be evaluated safely.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `recommendation-discovery`: Require eligible recommendation batches to be filled from the verified pool when enough candidates exist, and permit bounded variety while preserving prompt-fit and exclusion guarantees.

## Impact

- Affects the MusicBrainz/MCP discovery candidate ordering and the web recommendation selection/finalization path.
- Adds no external dependency, schema migration, or public endpoint change.
- Existing exclusion, verification, feedback logging, and mode-specific limits remain in force.
- Tests will use injected randomness or a seed so behavior is deterministic under test.