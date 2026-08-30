## Why

The Current Batch of recommended albums in the local web UI shows each album title as plain text, so there is no direct way to jump from a recommendation to the album's streaming page. Because the server already queries the iTunes Search API (for the release radar), it can resolve each recommended album to its Apple Music page with no new external service or authentication, and surface that link directly in the UI.

## What Changes

- When a recommendation batch is generated, resolve each recommended album to its **Apple Music album URL** by querying the iTunes Search API (artist + album, `entity=album`) and matching the top result.
- Persist the resolved streaming (Apple Music) URL on each stored `recommendation_candidate`.
- Include the resolved Apple Music URL in the `/api/recommendations` response payload for the current batch.
- Render the **Album Title** in the Current Batch list as a clickable link to the Apple Music page (opens in a new tab) whenever a URL is available; fall back to plain text when none could be resolved.
- **Scope — MVP only.** This change implements Apple Music links. A user-selectable streaming platform preference (intended to let others pick their service) is intentionally **out of scope** and deferred to a follow-up change; the URL-resolution design is kept isolated so additional platforms can be added later.

## Capabilities

### New Capabilities

- `streaming-links`: resolving external streaming-service URLs for recommended albums (Apple Music in this change) and presenting them as links in the web UI's Current Batch recommendation surface.

### Modified Capabilities

- *(none — `openspec/specs/` is currently empty and no committed capability spec changes its requirements.)*

## Impact

- **`internal/database/db.go`**: add a `streaming_url` (Apple Music) column to `recommendation_candidates` plus a schema-migration pass for existing databases.
- **`internal/database/recommendations.go`**: thread the resolved URL through `RecommendationCandidateInput` / `RecommendationCandidate` and the persist/read queries.
- **`internal/web/server.go`**: after candidate verification, resolve each candidate's Apple Music URL via the iTunes Search API and include it in the batch response; add the resolution provider to `Server`.
- **`internal/web/static/app.js`**: render the candidate album title as a link when a `streaming_url` is present.
- **Tests**: `internal/web/server_test.go` (URL resolution + response shape) and `internal/database/*` (column persistence).