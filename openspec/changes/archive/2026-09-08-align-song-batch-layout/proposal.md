## Why

On the dedicated Song Recommendations page, the Current Batch is separated from the prompt by a large empty workbench area, making the primary discovery loop feel disconnected. The song page also inherits the album-oriented default prompt, so the two recommendation modes do not communicate their different purposes clearly.

## What Changes

- Bring the song Current Batch into the primary content flow directly beneath the prompt, using the Album Recommendations tab as the layout reference.
- Preserve the song page's compact, scannable list and song-specific feedback actions while reducing unnecessary vertical separation and keeping the session/feedback sidecar usable.
- Give the Song Recommendations and Album Recommendations pages distinct default prompt text that matches each mode's output.
- Keep prompt restoration, latest-batch loading, explicit generation, and mode-isolated session history unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `recommendation-type-navigation`: Change dedicated-mode presentation so the song batch is visually adjacent to its composer and each dedicated mode has an appropriate default prompt.

## Impact

- Affected frontend assets: `/Users/nick/git/music-context-platform/internal/web/static/index.html`, `/Users/nick/git/music-context-platform/internal/web/static/app.js`, and `/Users/nick/git/music-context-platform/internal/web/static/app.css`.
- Affected frontend tests: embedded static-asset contract tests and any new mode-specific default/layout assertions in `/Users/nick/git/music-context-platform/internal/web/server_test.go`.
- No API endpoints, database schema, recommendation generation behavior, external dependencies, or persisted session data change.