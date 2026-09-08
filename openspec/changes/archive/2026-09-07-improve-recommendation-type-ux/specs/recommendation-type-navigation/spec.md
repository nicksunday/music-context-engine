## Purpose

Provides a clear shared entry point and navigation model for distinct song and album recommendation workflows, allowing each mode to use an appropriate page and session context.

## ADDED Requirements

### Requirement: Provide separate recommendation mode destinations

The Recommendation Engine MUST provide clearly labeled destinations for Song Recommendations and Album Recommendations instead of requiring the user to choose the primary mode from a dropdown. Both destinations MUST remain reachable from a shared recommendation entry surface.

#### Scenario: User opens the recommendation entry surface
- **WHEN** the user navigates to the Recommendation Engine entry surface
- **THEN** the user sees distinct, clearly labeled choices for Song Recommendations and Album Recommendations

#### Scenario: User opens a recommendation mode
- **WHEN** the user selects either recommendation mode
- **THEN** the system opens that mode's dedicated page with its mode-appropriate session and presentation

### Requirement: Provide shared navigation between recommendation modes

Each dedicated recommendation page MUST provide consistent navigation to the other recommendation mode and to the shared Recommendation Engine entry surface. Switching modes MUST NOT silently merge or replace the current session history of the destination mode.

#### Scenario: Switch from songs to albums
- **WHEN** a user selects Album Recommendations from the shared navigation on a song page
- **THEN** the album page opens and the user's album session context is shown independently of the song session context

#### Scenario: Return to the recommendation entry surface
- **WHEN** a user activates the shared Recommendation Engine navigation
- **THEN** the entry surface opens without generating a recommendation batch or changing either mode's persisted session data

### Requirement: Render recommendation modes with appropriate presentation

The song page MUST render candidates as a compact, scannable list with a direct destination link when available. The album page MUST retain a richer card-based presentation with album metadata and album-level actions.

#### Scenario: Song batch is displayed
- **WHEN** a song recommendation batch is available
- **THEN** each candidate is displayed as a list item containing its song identity and an actionable direct link when a destination exists

#### Scenario: Album batch is displayed
- **WHEN** an album recommendation batch is available
- **THEN** candidates are displayed using the album-oriented card presentation and album feedback controls