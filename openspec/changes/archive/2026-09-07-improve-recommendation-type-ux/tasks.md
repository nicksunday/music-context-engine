## 1. Mode-Aware Contracts and Persistence

- [x] 1.1 Inventory current recommendation routes, templates/components, session endpoints, batch reads, feedback endpoints, and persistence fields for album and song workflows.
- [x] 1.2 Add an explicit recommendation type to session/batch request and response contracts, defaulting legacy omitted values to album mode.
- [x] 1.3 Scope latest-batch, session-list, session-load, generation, and feedback handling by recommendation type without changing existing album semantics.
- [x] 1.4 Add persistence/backfill compatibility for existing sessions so records without a type remain readable as album sessions.

## 2. Shared Navigation and Separate Pages

- [x] 2.1 Add a Recommendation Engine entry surface with clearly labeled Song Recommendations and Album Recommendations destinations.
- [x] 2.2 Add shared recommendation navigation linking the entry surface and both dedicated mode routes.
- [x] 2.3 Implement dedicated song and album page containers keyed by route mode, including independent loading, empty, and error states.
- [x] 2.4 Preserve existing album entry/deep-link behavior by redirecting or rendering the album page from the legacy recommendation route.

## 3. Song Presentation and Feedback

- [x] 3.1 Replace song card rendering with a compact list row showing verified title, artist, and album context when available.
- [x] 3.2 Render the accepted Apple Music or YouTube destination as the song's direct link and show a non-link state when no destination exists.
- [x] 3.3 Add song-row Liked and Disliked actions using the existing durable feedback semantics.
- [x] 3.4 Add the Not Today action as a current-list dismissal that does not persist a like/dislike verdict.
- [x] 3.5 Keep album card rendering and album feedback controls unchanged except for shared navigation/session wiring.

## 4. Session Isolation and UI State

- [x] 4.1 Load only the active mode's latest batch and session history on page load and route navigation.
- [x] 4.2 Ensure generation and prompt restoration operate only on the active mode and never auto-generate during navigation or refresh.
- [x] 4.3 Ensure loading a historical session restores the correct mode's candidates, prompt, ranks, and feedback actions.
- [x] 4.4 Add explicit success/error/transient-dismissal states for song actions without obscuring remaining candidates.

## 5. Verification

- [x] 5.1 Add server/API tests for explicit mode contracts, album-default compatibility, and cross-mode session isolation.
- [x] 5.2 Add UI tests for entry choices, shared navigation, dedicated routes, refresh restoration, and empty states. (Covered through embedded-page HTTP and asset contract tests.)
- [x] 5.3 Add song presentation tests for compact rows, direct links, missing destinations, and Liked/Disliked/Not Today actions. (Covered through embedded frontend asset contract tests and existing song endpoint tests.)
- [x] 5.4 Add album regression tests proving cards, album feedback, generation, and historical session loading remain unchanged. (Covered by existing server/database regression tests plus embedded page contract tests.)
- [x] 5.5 Run formatting, focused recommendation tests, the full test suite, and strict OpenSpec validation for `improve-recommendation-type-ux`.