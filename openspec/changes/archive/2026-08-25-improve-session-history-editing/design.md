## Context

The web UI (see `internal/web/static/index.html` + `app.js`) renders each past session as a `<button class="session-row">` whose entire content — prompt, metadata, and reply — is inside the button. Clicking it calls `loadSession(id)` → `GET /api/batch?id=` → `setCurrentBatch`. Reason: (1) browsers typically do not let users select/copy text inside a `<button>`, so the recorded prompt/reply can't be re-read; (2) the only action is a heavy "load full batch" click, and if that request fails the code just appends an error message — the prompt is never restored and there is no clear failure state, so from the user's perspective "loading a session does nothing." The server read endpoints required by `recommendation-session-history` (list sessions, get latest batch, get batch by id) already exist and already return the prompt. See proposal.md - Why.

## Goals / Non-Goals

**Goals:**
- Let users select and copy the text of any past session (prompt, date/count meta, reply) directly in the side panel.
- Give every session an explicit "restore prompt" action that puts that session's prompt back into the Prompt textarea (and its reply read-only) without generating a new batch.
- When a full batch load fails, keep the session usable: restore the prompt and show an explicit, focused error state instead of a silently unchanged batch.

**Non-Goals:**
- Re-authoring the persistence or read endpoints' response shape (no new endpoints; the prompt is already in the list payload).
- Changing verdict feedback, ranking, or candidate structure.
- Replacing the existing full-batch load behavior — restore-prompt is additive, load remains the primary action.

## Decisions

**D1: Stop wrapping session content in a `<button>`; make the row a non-interactive container with explicit selectable children, and put replay/restore in separate controls.**
The whole row is currently a `<button>`, which kills text selection. Restructure each row as a `<div class="session-row">` that contains selectable text nodes plus two small controls: a "Load" button that preserves today's full-batch load, and a "Restore prompt" button.
*Alternatives:* keep the row a button and toggle `user-select: text` via CSS — rejected because browsers still apply selection heuristics to button internals and it conflates the copy affordance with the load action.
- *Why selectable text:* the user explicitly asked to be able to highlight session text; this is the least surprising way to satisfy it.
- *Why a separate restore action:* the user asked for "either prompt into the text box or highlightable text"; implementing restore as an explicit control keeps the load action and guarantees the prompt can always come back, even when the batch fetch fails.

**D2: Reuse the session object already present in the rendered list for restore; only full-batch load needs the network.**
Each listed session already carries `prompt`, `mood`, and `created_at` from `GET /api/batches` (server.go `handleBatches`). "Restore prompt" can therefore be fully client-side: write `session.prompt` (and mood, if present) into `#message` / `#mood` and focus the prompt textarea. No new server call and no new payload fields. Only the existing "load" action keeps calling `GET /api/batch?id=`.
- *Why:* simplest, works off the current API, and makes restore resilient to batch-load failures.

**D3: Make load-failure self-documenting and partial.** `loadSession` currently swallows failures into a generic error line. Change it so a failure: (a) restores that session's prompt into the text box (same path as D2), (b) renders an explicit "could not load this session" empty/error state in the Current Batch surface, and (c) re-raises a focused message. Tag the batch surface as stale rather than silently leaving a different batch on screen (the user already finds "does nothing" confusing).
- *Why restore prompt on failure:* it is exactly the recovery the user asked for and it degrades gracefully.
- *Why an explicit error state:* a silent no-op is the reported bug; the surface must make the outcome visible.

**D4: Enable clean text selection.** Apply `user-select: text` (with the `-webkit-` prefix) to the row's text nodes in `app.css`, ensure the row layout uses normal document text (no `pointer-events` traps on the text container), and keep the row's internal buttons as the only interactive elements so clicks don't swallow selection drags.

## Risks / Trade-offs

- Replacing the row's `<button>` with a `<div>` + inner buttons changes document semantics (no longer a single-button tab target). → Give each row a single "loading" control with keyboard focus and keep the row focusable as a whole via `tabindex`/ARIA grouping; keyboard Enter on the focused row triggers load.
- Dependency: restore-of-prompt relies on list payload already containing `mood`. If a given session's mood is empty/absent, the restore fills just the prompt. → Acceptable; prompt restore always works, mood restore is best-effort.
- Making text selectable could slightly reduce the perceived click area for loading a batch (drag vs. click). → Keep the row-level load click on the container (non-text padding) while the inner "Restore prompt" button is a precise target; text drags select/copy instead of triggering load.

## Migration Plan

- Frontend-only change within `internal/web/static/`; no server, schema, or data migration.
- Rollback: revert the app.js/app.css/index.html changes; endpoints unchanged so old and new clients interoperate.

## Open Questions

None — deferred decisions do not change the specs, approach, or task breakdown.