## Context

The current web recommendation experience serves album and song batches through a shared page and mode control. Existing recommendation data already distinguishes song candidates, album candidates, destination providers, feedback, and persisted sessions, but the UI/session reads need an explicit recommendation type so separate pages do not display each other's latest batch or history. See `proposal.md` for motivation and the change specs for the observable contract.

## Goals / Non-Goals

**Goals:**
- Introduce stable song and album routes/pages behind a shared recommendation navigation shell.
- Reuse generation and verified candidate APIs while adding explicit type scoping to session reads and writes.
- Render songs as a compact list and albums as cards without weakening existing album behavior.
- Implement Not Today as a current-view dismissal, distinct from persisted like/dislike feedback.
- Preserve refresh, restore, empty, loading, and error behavior independently for each mode.

**Non-Goals:**
- No change to recommendation quality, candidate verification, provider lookup rules, or album feedback semantics.
- No playlist creation or streaming account integration.
- No user-facing preference or configuration for choosing the default recommendation mode.
- No destructive migration of existing album sessions; legacy album records remain readable as album sessions.

## Decisions

### D1. Use explicit mode routes with a shared navigation shell

Expose an entry route plus dedicated song and album routes, with shared links/tabs in the recommendation shell. Route identity is the source of truth for the active mode rather than a dropdown value. This makes the two available workflows visible and enables independent loading and restoration.

*Alternative considered:* keep one route and replace the dropdown with tabs. Rejected because separate routes make browser history, refresh behavior, deep links, and session isolation unambiguous.

### D2. Scope session reads and generation requests by recommendation type

Carry an explicit recommendation type through latest-batch, session-list, session-load, generation, and feedback interactions. Treat existing records without a type as album records for backward compatibility. The server remains authoritative for validating the type and preventing cross-mode loads.

*Alternative considered:* filter only in the browser. Rejected because it risks leaking the other mode's history and creates incorrect behavior for direct API calls or refreshes.

### D3. Keep song Not Today transient and local to the visible batch

Handle Not Today as a dismissal/removal from the active client-side list, without writing a durable like/dislike verdict. A later generated or reloaded batch may show the candidate again according to existing recommendation/exclusion rules.

*Alternative considered:* persist a third feedback verdict. Rejected because the requested behavior is temporary dismissal and a new durable state would alter recommendation filtering semantics.

### D4. Use separate rendering components over shared candidate primitives

Share loading, error, session, generation, and feedback plumbing where possible, but keep song-row and album-card rendering separate. Song rows prioritize title, artist, direct link, and three quick actions; album cards retain metadata, notes, and current album controls.

*Alternative considered:* make cards responsive enough to represent both. Rejected because the compact song workflow would inherit unnecessary album density and interaction cost.

### D5. Preserve API compatibility while adding type fields

Add explicit type fields/parameters in a backward-compatible way, defaulting omitted legacy reads and writes to album mode where the existing endpoint represented album sessions. New UI requests always send the explicit type. Response shapes retain existing candidate fields and add only mode/session metadata needed for isolation.

## Risks / Trade-offs

- [Legacy session rows may not have a stored type] → Treat them as albums and add migration/backfill coverage without changing their candidate data.
- [A shared shell can accidentally retain stale state across route changes] → Key page data and requests by active mode and reload mode-scoped latest/session data on navigation.
- [Not Today dismissal may make a user think feedback was saved] → Use clear transient affordance/state and do not show it as a durable verdict in session history.
- [More routes create duplicate UI states] → Centralize shared fetch, submit, and error handling while keeping only the presentation and action differences mode-specific.
- [Existing deep links may target the old page] → Preserve the old entry route as a landing/redirect-compatible surface and verify album behavior through regression tests.

## Migration Plan

1. Add mode-aware server/API contracts and compatibility defaults without changing existing album records.
2. Add route-level shared navigation and dedicated song/album page containers.
3. Move song rendering and feedback into the compact list workflow; keep album cards and controls unchanged.
4. Update session persistence/read paths and add coverage for legacy album rows and cross-mode isolation.
5. Add browser/server tests for navigation, refresh, generation, session restore, direct links, and all song feedback actions.
6. Roll back by routing both modes through the existing page and disabling the new mode-specific reads; no destructive data rollback should be required.