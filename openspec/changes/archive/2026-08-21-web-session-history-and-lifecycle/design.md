## Context

The web UI's Current Batch lives only in `state.currentBatch` (`internal/web/static/app.js`); on page load it calls `renderBatch(null)` and shows "No current batch", even though batches are fully persisted by `database.CreateRecommendationBatch` into `recommendation_batches`/`recommendation_candidates`. There is no database function that reads a batch back and no HTTP endpoint for it. `recommendation_batches` already stores `prompt`, `mood`, `notes` (the reply) and `created_at`, so the raw data for a browsable session list is already present.

`cmd/music-vault/main.go` `runWeb` starts with a bare `http.ListenAndServe(addr, handler)` and then `log.Fatalf` on any returned error — there is no `http.Server.Shutdown`, no `signal.NotifyContext`, and the SQLite handle is closed only via `defer db.Ctx.Close()`. See proposal.md — Why for the motivation and the two capabilities.

## Goals / Non-Goals

**Goals:**
- Restore the latest persisted batch in the Current Batch surface on page load, without generating a new one.
- Add a read-only endpoint for the latest batch and a read-only endpoint for the past-session list (plus a by-id batch fetch for loading older sessions).
- Render past sessions in the UI and allow loading a selected session's batch back into the Current Batch surface.
- Stop/restart the web server gracefully via SIGINT/SIGTERM using `http.Server.Shutdown`, closing the DB handle and draining in-flight requests.

**Non-Goals:**
- No changes to generation logic, the persisted schema, the MCP server, or existing endpoints' contracts.
- No user accounts, auth, or a remote HTTP "stop" endpoint (shutdown is signal-driven to keep the localhost surface simple).
- No auto-generation of batches, on refresh or otherwise.
- No pagination of the session list beyond a bounded recent window in this change.

## Decisions

**D1. Read back batches with new database functions rather than re-querying in the handler.**
Add `FetchLatestRecommendationBatch` and `ListRecommendationBatches` (and a by-id `FetchRecommendationBatch`) to `internal/database/recommendations.go`, reusing the same row-scanning/mapping already used by `CreateRecommendationBatch`. The server handlers stay thin.
- *Why:* keeps SQL/row-scanning in the database layer consistent with existing code (e.g., `FetchRecentRecommendationFeedback`), and makes both queries unit-testable without an HTTP layer.
- *Alternative considered:* parsing joined rows directly in the `web` handler — rejected, mixes DB concerns into the handler and duplicates mapping.

**D2. Two/three GET endpoints: `GET /api/batch/latest`, `GET /api/batches`, `GET /api/batch?id=<id>`.**
- `GET /api/batch/latest` returns the single most recent `RecommendationBatch` (its candidates included) or an explicit `null`/empty when none exists.
- `GET /api/batches` returns a bounded list of sessions (prompt, mood, created_at, candidate count, reply), newest first, each with its `batch_id`.
- `GET /api/batch?id=<id>` returns one batch by id (used to load a selected older session's full candidates).
- *Why:* a lightweight list plus latest/by-id fetches keeps payloads small and reuses the batch JSON shape the Current Batch surface already consumes.
- *Alternative:* one endpoint returning full candidates for every session — rejected, heavier payloads and muddier semantics.

**D3. Frontend loads latest batch on startup and holds sessions in state.**
`app.js` adds a startup step (after `refreshAll`) that fetches `/api/batch/latest` and renders it, and fetches `/api/batches` to populate a new past-sessions panel. Selecting a session fetches that batch by id and re-renders it in the Current Batch surface.
- *Why:* mirrors the existing read pattern, keeps the sessions list lightweight, and reuses render logic.

**D4. Graceful shutdown via `signal.NotifyContext` + `http.Server.Shutdown`.**
`runWeb` constructs an `http.Server{Addr, Handler}`, creates a context cancelled on SIGINT/SIGTERM, runs `ListenAndServe` in a goroutine, and on signal calls `server.Shutdown(graceCtx)` with a short timeout; it then closes the DB and returns normally. `ErrServerClosed` from `ListenAndServe` is treated as a normal exit, not a fatal error.
- *Why:* the standard Go idiom; `Shutdown` stops accepting connections and drains in-flight requests, then we close the SQLite handle — satisfying the lifecycle spec.
- *Alternative considered:* an HTTP stop endpoint — rejected (non-goal, would add an unauthenticated control surface on localhost).

## Risks / Trade-offs

- [Requests in flight during shutdown] → `Shutdown` with a bounded grace timeout drains them; stragglers are then cancelled, acceptable for a local tool.
- [The latest batch is not necessarily the currently displayed one if an older session was reloaded] → by design, `/api/batch/latest` returns the newest persisted batch regardless of which session is displayed; the session panel makes the loaded one explicit. Noted as a UX trade-off, not a bug.
- [Session list grows over time] → bounded recent window (e.g., last N sessions) returned by `GET /api/batches`; full history stays in the DB. Acceptable for MVP.

## Migration Plan

1. Add the read-only database functions and handlers/endpoints; they are additive and safe against an existing DB (no schema change).
2. Wire graceful shutdown in `runWeb`; Ctrl+C, `kill -INT`, and `kill -TERM` become clean without any data migration.
3. Update the frontend to load latest batch + sessions on startup.
4. Rollback: revert frontend startup fetch and/or shutdown wiring; read endpoints are additive and harmless to leave.

## Open Questions

- Whether "restart" implies an automated restart loop (supervisor) or just clean stop-then-start; the design implements clean stop via signals, which is the prerequisite either way.