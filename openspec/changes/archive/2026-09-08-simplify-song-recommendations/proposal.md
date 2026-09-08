## Why

Song Recommendations repeats the entry-page choice cards, exposes abandoned Apple Music playlist controls, and squeezes tracks into a two-column card grid. The page should focus on scanning songs and giving feedback.

## What Changes

- Show workflow choice cards only on the Recommendation Engine entry page.
- Remove Apple Music authorization, playlist naming, creation, and retry UI and browser integration.
- Render songs as flat single-column rows with readable identity, direct links, and existing feedback actions.
- Preserve the album card layout and direct Apple Music song links.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `recommendation-type-navigation`: Limit choice cards to entry and require flat single-column song presentation.
- `song-recommendations`: Exclude authorization and playlist creation controls from the song workflow.

## Impact

Web static HTML, CSS, JavaScript, and relevant web validation. Existing backend playlist endpoints are outside this UI cleanup; no data migration or new dependencies.
