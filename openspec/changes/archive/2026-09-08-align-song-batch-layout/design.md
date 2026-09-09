## Context

The shared frontend template currently renders the composer before a full-height workbench. `app.js` already derives `state.mode` from the URL and applies song-specific candidate rendering, while `app.css` already distinguishes `.song-list`/`.song-row` from album cards. The change should use those existing mode hooks rather than introducing separate templates or a new layout framework. See `proposal.md` for motivation and the delta spec for observable behavior.

## Goals / Non-Goals

**Goals:**

- Make the prompt-to-batch relationship immediate on both dedicated recommendation pages, especially for songs.
- Preserve the existing album card treatment and song list treatment.
- Set mode-specific defaults only when the page is initialized, while allowing restored and edited prompts to win.
- Keep the sidecar available without allowing it to create a large blank region above results.

**Non-Goals:**

- Changing recommendation APIs, batch persistence, session loading, or feedback semantics.
- Redesigning the shared entry page beyond any CSS/layout rules required by the dedicated pages.
- Adding client-side routing, a frontend dependency, or separate HTML documents per mode.

## Decisions

### Use existing mode state to select defaults

Keep one shared `index.html` and define the song and album default strings in the existing mode initialization path in `app.js`. Apply a default only when the prompt still contains the template's initial value (or is otherwise uninitialized), so restored session prompts and explicit user edits are not overwritten. This is preferred over server-rendered mode-specific HTML because the static page is already shared and the URL is the established mode source.

### Let the workbench size to content on dedicated pages

Adjust the dedicated-page grid and workbench sizing so results follow the composer naturally instead of reserving a viewport-sized empty region before the batch. Keep the existing results/sidecar columns and responsive breakpoint; only change the row sizing, gaps, and alignment needed to keep the Current Batch adjacent to the prompt. This is preferred over moving batch markup into the form because it preserves the current semantic sections and existing rendering hooks.

### Preserve mode-specific candidate classes

Continue to toggle the candidate list's song class from `app.js`, and scope any layout refinements through existing mode classes or dedicated-page selectors. Album cards remain unchanged except for their position in the tighter flow.

## Risks / Trade-offs

- [Risk] A content-sized workbench may reduce the amount of simultaneous content visible on very tall screens. → Mitigation: retain the existing scrollable result/list regions and responsive behavior, and verify both empty and populated states.
- [Risk] Default selection could overwrite a prompt restored during startup. → Mitigation: set defaults before asynchronous session restoration and only replace the known template default, never non-empty user/session text.
- [Risk] CSS changes could affect the shared Recommendation Engine entry page. → Mitigation: scope dedicated-page rules to the active mode and add static contract tests for the mode hooks.

## Migration Plan

No data or API migration is required. Deploy the static asset changes together; existing persisted batches and prompts remain compatible. Rollback consists of reverting the frontend assets.