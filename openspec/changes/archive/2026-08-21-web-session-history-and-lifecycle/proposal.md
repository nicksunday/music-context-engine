## Why

The web UI keeps the Current Batch only in browser memory, so a page refresh wipes it even though batches are already persisted in SQLite — there is no way to reload the latest batch or revisit an earlier one. Separately, the web server runs a bare `http.ListenAndServe` with no shutdown handling, so there is no clean way to stop or restart it.

## What Changes

- **Graceful server shutdown / restart:** `runWeb` switches to an `http.Server` with `Server.Shutdown(ctx)` and OS signal handling (SIGINT/SIGTERM, i.e. Ctrl+C and `kill -INT`/`kill -TERM`), so the server drains in-flight requests, closes the SQLite handle, and exits cleanly instead of being hard-killed.
- **Current batch survives page refresh:** the server exposes a `GET /api/batch/latest` endpoint that returns the most recently persisted batch (with its candidates), and the frontend loads and renders it on page load instead of starting empty. Generation still happens **only** when the user presses Generate Batch (POST `/api/recommendations`) — no auto-generation on refresh.
- **Previous session prompts are browsable:** the server exposes a `GET /api/batches` endpoint listing recent sessions (prompt, created date, candidate count, reply) from `recommendation_batches`, and the web UI shows these past sessions and lets the user load a selected session's album batch back into the Current Batch surface.
- **No breaking changes:** existing endpoints and the persisted schema are unchanged; only additive read endpoints, server lifecycle wiring, and frontend behavior are introduced.

## Capabilities

### New Capabilities

- `recommendation-session-history`: persisting the current recommendation batch to the database such that it can be reloaded after a page refresh, exposing read endpoints for the latest batch and a browsable list of past session prompts, and rendering both in the web UI (with generation still triggered only by the user).
- `web-server-lifecycle`: graceful shutdown and clean restart of the local `music-vault web` server via `http.Server.Shutdown` and OS signal handling (SIGINT/SIGTERM).

### Modified Capabilities

- *(none — the `streaming-links` capability's requirements are unchanged.)*

## Impact

- **`cmd/music-vault/main.go`**: replace `http.ListenAndServe` with `http.Server` + `signal.NotifyContext` + graceful `Shutdown`; import `os`/`os/signal`/`syscall`/`context`/`errors` as needed.
- **`internal/web/server.go`**: register `GET /api/batch/latest` and `GET /api/batches` handlers; add server methods to fetch the latest batch and list sessions.
- **`internal/database/recommendations.go`**: add `FetchLatestRecommendationBatch` and `ListRecommendationBatches` functions (read-only, joining `recommendation_batches` with their candidates).
- **`internal/web/static/app.js`** and **`internal/web/static/index.html`**: load and render the latest batch on startup; add a past-sessions panel that lists prompts and loads a selected session's batch.
- **Tests**: database (latest-batch + list functions), web server (new GET endpoints), and `cmd/music-vault` shutdown behavior.
