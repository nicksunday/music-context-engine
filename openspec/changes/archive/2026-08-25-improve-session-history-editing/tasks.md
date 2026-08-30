## 1. Session row markup & rendering

- [x] 1.1 In `internal/web/static/app.js` `renderSessions`, restructure each row from a single `<button class="session-row">` into a `<div class="session-row" tabindex="0" role="group">` containing separate selectable text nodes for prompt, metadata, and reply.
- [x] 1.2 Add a "Load" control inside each row that preserves today's action: on click call `loadSession(session.id)` and render its failure via the new error path (taskgroup 3).
- [x] 1.3 Keep the row container keyboard-accessible and trigger the load action on Enter (keyboard) so tab/enter parity with the old button is preserved.
- [x] 1.4 In `internal/web/static/index.html`, keep the `#sessionList` container and confirm the `.session-row` semantics still fit the rail markup; adjust class names if needed.

## 2. Restore prompt into the input

- [x] 2.1 Add a `restorePrompt(session)` helper in `app.js` that writes `session.prompt` into the `#message` textarea and `session.mood` into `#mood` when present, then focuses the prompt textarea — client-side only, no network call (see design D2).
- [x] 2.2 Wire a "Restore prompt" button into each session row (next to the Load control) that calls `restorePrompt(session)` and does NOT generate a new batch.
- [x] 2.3 Confirm a restored prompt is fully editable and can later be submitted normally via Generate Batch.

## 3. Load-failure behavior

- [x] 3.1 In `app.js` `loadSession`, on failure call `restorePrompt(session)` so the prompt still comes back, then render an explicit "could not load this session" state in the Current Batch surface (mark it stale) instead of silently leaving it unchanged (see design D3).
- [x] 3.2 Keep the existing error line/`appendMessage` output but make it specific to the failed session load so the user sees a clear, non-generic message.
- [x] 3.3 Ensure a failed load never auto-generates a batch and never mutates persisted data (read-only, consistent with the capability's "read endpoints must not mutate state").

## 4. CSS for selectable text

- [x] 4.1 In `internal/web/static/app.css`, apply `user-select: text` (with `-webkit-user-select` prefix) to the session row text nodes so prompt/meta/reply can be highlighted and copied.
- [x] 4.2 Keep the row's inner buttons interactive and ensure text drags select/copy instead of triggering the row's load action (check `pointer-events` on the row container text area; see design D4).
- [x] 4.3 Preserve the existing `.session-row` visual styling (background, hover/focus states) after the button→div restructure.

## 5. Verification

- [x] 5.1 Run `go build ./...` and `go test ./internal/web/...` from the repository root to confirm no regression in web server behavior.
- [x] 5.2 Manually verify in the running app: highlight/copy a past session's prompt and reply; click "Restore prompt" and confirm the text box is populated; load a session and confirm the batch renders; and confirm a failed load shows the explicit error and still restores the prompt.
- [x] 5.3 Confirm restore does not auto-submit/generate a new batch and that a restored prompt can then be edited and submitted normally.
- [x] 5.4 Re-run `openspec validate --change improve-session-history-editing` to confirm the change specs still validate.