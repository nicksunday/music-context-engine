## Why

Album recommendations can currently feel repetitive within a batch, and comparison-style prompts can be treated as loose tag hints instead of decomposed musical references. This change tightens album-mode discovery so small batches have more artist variety and prompts like "similar to X", "X but heavier", or "like X but less Y" preserve the intended tone across genres.

## What Changes

- Prefer artist variety in default album recommendation batches by selecting no more than one album per normalized artist unless the user's prompt explicitly asks for a specific artist, catalog, or deep dive.
- Keep the existing album-level exclusion policy: known artists remain eligible, but exact rated or feedback-blocked albums stay excluded.
- Add comparison-aware prompt planning for named artists, styles, and "like/similar/vibes/but/less/more" phrasing.
- Preserve comparison dimensions through tag planning and ranking: reference anchors, intended traits, required traits, flexible traits, false-friend traits, and acceptable bridge traits.
- Rank candidates by how well they satisfy the comparison's musical intent, not only by broad genre-tag overlap or historical taste affinity.
- Keep Symphony X / Children of Bodom-style virtuosic metal as one regression fixture, not the whole design center.
- Preserve verified-candidate grounding: Ollama still selects from MCP-verified candidates and cannot invent displayed albums, artists, or starter tracks.
- Add focused tests for artist diversity and cross-genre comparison quality.

## Capabilities

### New Capabilities

- `recommendation-discovery`: refined album-batch diversity and comparison-aware prompt-tone fit for verified recommendations. This change targets the same capability currently being migrated from `docs/specs/09_recommendation_discovery.md`.

### Modified Capabilities

- *(none — `recommendation-discovery` is not yet archived under `openspec/specs/`; it exists in the active docs-migration change.)*

## Impact

- **`internal/mcp/discovery.go`**: preserve the verified discovery candidate pool and its existing duplicate album suppression so web ranking still has enough options.
- **`internal/web/server.go`**: update discovery planning, candidate ranking, fallback selection, and final batch filtering to enforce displayed-batch artist variety and comparison-aware tone fit.
- **`internal/web/server_test.go` / `internal/mcp/discovery_test.go`**: add coverage for one-per-artist displayed batches, explicit repetition opt-in, and comparison-aware ranking while preserving MCP candidate-pool behavior.
- **Specs/docs**: reconcile the active recommendation-discovery spec wording with the refined diversity and comparison-aware recommendation contract.
