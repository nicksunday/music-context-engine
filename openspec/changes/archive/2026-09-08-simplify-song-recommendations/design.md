## Context

See proposal.md for motivation. The shared HTML sets modeChooser.hidden correctly, but .mode-chooser declares display:grid, overriding browser hidden styling. Song rows also inherit the shared candidate grid. The browser includes MusicKit authorization and playlist code. Existing unrelated workspace edits must be preserved.

## Goals / Non-Goals

Goals: Fix visibility and song layout at their shared CSS/rendering boundaries; remove abandoned browser playlist integration completely.
Non-goals: Backend playlist API removal, data migrations, recommendation ranking changes, or album redesign.

## Decisions

- Enforce the HTML hidden attribute in shared CSS rather than special-casing song routes, keeping entry and album behavior consistent.
- Give song batches a dedicated single-column list class; remove row card decoration and use simple separators. Wrap actions beneath identity on narrow screens. Preserve the album grid.
- Remove playlist controls, MusicKit loading, state, handlers, and retry UI rather than merely hiding buttons. Preserve direct provider-link helpers independently of account authorization.

## Risks / Trade-offs

- Shared CSS can affect albums → verify all three routes and album card columns.
- Removing shared JavaScript can break initial rendering → check startup, empty/current/retained batches, direct links, and feedback.
- Backend playlist endpoints remain present → this change explicitly retires the browser workflow only.

## Migration Plan

Rebuild the embedded web assets and restart the local web server when applying. No database migration. Rollback restores the affected static assets.
