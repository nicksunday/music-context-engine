## Why

The discovery engine currently treats known artists, track titles, and local-library album names as global exclusion tokens, which blocks valid "new-to-me album" recommendations from artists the user already knows. This over-filters the candidate pool and pushes prompts toward more obscure adjacent tags instead of strong unrated albums from familiar artists.

## What Changes

- Change the default discovery exclusion policy from a flat artist/album/track blacklist to exact album-level exclusion.
- Do not exclude a candidate solely because its artist is already present in the local library.
- Exclude exact artist+album pairs that have a whole-album `user_rating` from the numeric album-rating flow.
- Exclude exact artist+album pairs with durable web feedback verdicts: `disliked`, `ok`, `good`, `great`, and `already_know`.
- Treat `not_for_me_today` as a same-local-day cooldown only; after that local calendar day, the album is eligible again unless it also has a numeric album rating or durable feedback verdict.
- Keep albums eligible when the only local evidence is track-level history, including liked/favorited songs, unless the album itself has a numeric rating, durable feedback verdict, or same-day `not_for_me_today` cooldown.
- Preserve per-batch duplicate suppression and the at-most-two-candidates-per-artist diversity guard.

## Capabilities

### New Capabilities

- `recommendation-discovery`: album-first discovery candidate exclusion semantics for verified recommendations. This change targets the same capability currently being migrated from `docs/specs/09_recommendation_discovery.md`.

### Modified Capabilities

- *(none — `recommendation-discovery` is not yet archived under `openspec/specs/`; it exists in the active docs-migration change.)*

## Impact

- **`internal/database/analytics.go`**: replace or supplement the flat string exclusion map with an album-level exclusion set keyed by normalized artist+album pairs and feedback verdict/timestamp rules.
- **`internal/mcp/discovery.go`**: change verified discovery filtering so known artists and starter tracks are not hard-blocked by default; only excluded albums are removed.
- **`internal/web/server.go`**: align the post-model filtering path with the album-level exclusion policy.
- **Specs/docs**: reconcile the existing recommendation-discovery wording that says artist/track tokens are added to exclusions.
- **Tests**: update database, MCP discovery, and web recommendation tests to cover known-artist unrated albums, numeric rating blocking, durable feedback blocking, and same-day-only `not_for_me_today` blocking.
