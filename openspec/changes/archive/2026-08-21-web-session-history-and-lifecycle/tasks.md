## 1. Database: read back batches and sessions

- [x] 1.1 Add `FetchLatestRecommendationBatch(ctx, db)` to `internal/database/recommendations.go` returning the most recent `RecommendationBatch` (with candidates) or a zero value when none exists.
- [x] 1.2 Add `FetchRecommendationBatchByID(ctx, db, id)` returning a single batch's candidates (used to load a selected older session).
- [x] 1.3 Add `ListRecommendationBatches(ctx, db, limit)` returning recent sessions (id, prompt, mood, created_at, candidate count, reply) newest first.
- [x] 1.4 Add `internal/database/recommendations_test.go` tests for latest/by-id fetch and list (empty DB, populated DB, ordering).

## 2. Web server: read-only batch/session endpoints

- [x] 2.1 Register `GET /api/batch/latest`, `GET /api/batch?id=<id>`, and `GET /api/batches` handlers in `NewServer` (`internal/web/server.go`).
- [x] 2.2 Implement the latest-batch handler (returns the batch JSON or `{"batch": null}` when none exists).
- [x] 2.3 Implement the by-id batch handler (404 with a friendly error when the id is unknown).
- [x] 2.4 Implement the batches-list handler (returns a bounded session list with prompt/mood/date/count/reply).
- [x] 2.5 Ensure all three handlers are read-only (no inserts/updates/deletes).

## 3. Web server: graceful shutdown

- [x] 3.1 In `cmd/music-vault/main.go` `runWeb`, replace `http.ListenAndServe` with an `http.Server` plus `signal.NotifyContext` (SIGINT/SIGTERM).
- [x] 3.2 Run `ListenAndServe` in a goroutine and, on signal, call `server.Shutdown(graceCtx)` with a bounded grace timeout; treat `http.ErrServerClosed` as a normal exit (not fatal).
- [x] 3.3 Close the SQLite DB handle after shutdown and return cleanly from `runWeb`.

## 4. Frontend: restore the latest batch on page load

- [x] 4.1 In `internal/web/static/app.js`, on startup fetch `/api/batch/latest` and render it (or an explicit empty state when no batch exists).
- [x] 4.2 Confirm generation still only occurs via the Generate Batch form (no fetch triggers auto-generation on load/refresh).

## 5. Frontend: browsable past sessions

- [x] 5.1 Fetch `/api/batches` on startup and render a past-sessions panel (prompt, date, candidate count, reply).
- [x] 5.2 Add UI to select a past session; fetch that batch by id and render its candidates in the Current Batch surface with verdict buttons operable.
- [x] 5.3 Show an explicit empty state when there are no prior sessions.

## 6. Tests & validation

- [x] 6.1 Add `internal/web/server_test.go` tests for the latest/batch-id/list endpoints (empty DB, populated DB, unknown id).
- [x] 6.2 Add a `cmd/music-vault` (or equivalent) test/verification that a graceful shutdown path closes the server without a fatal error on signal.
- [x] 6.3 Run `go build ./...`, `go vet ./...`, and `go test ./...`; confirm `git status` shows only intended changes.
