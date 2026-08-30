## Why

The session history side panel records previous recommendation sessions, but it isn't usable for recovery: selecting a past session does not reliably restore its content, and the session text cannot be copied. As a result users cannot reuse or reference what they previously asked for. When loading fails, the only thing they want is to get the session prompt back — either restored into the prompt box or selectable so they can copy it themselves.

## What Changes

- Add an explicit way to restore a past session's prompt into the prompt text box (e.g. a "restore prompt" affordance that only repopulates the input), separate from and tolerant of the batch-loading path.
- Make session history text user-selectable and copyable so users can highlight and copy a past prompt, reply, or candidate text regardless of whether batch loading succeeds.
- Fix the session load interaction in the side panel so that loading a selected session responsively reflects the chosen session (candidates, ranks, prompt) or shows a clear failure state instead of silently doing nothing.

## Capabilities

### New Capabilities
- *(none)*

### Modified Capabilities
- `recommendation-session-history`: The existing "Browse previous session prompts" and "Load a selected past session's batch" requirements gain explicit support for restoring a session's prompt into the prompt input and for making session text selectable/copyable, with a defined behavior when loading fails (partial restore).

## Impact

- Frontend web UI components: session history panel, prompt input, current batch surface, and the client code that calls the read endpoints.
- Server read endpoints for listing sessions and loading a session's batch, if needed to support partial restore (the prompt is already part of the session list payload).
- No new persistent data or schema changes; this builds on the existing session persistence.
- No breaking API changes.
- Tests: verify restore-into-input, text selectability, and failure-state behavior.