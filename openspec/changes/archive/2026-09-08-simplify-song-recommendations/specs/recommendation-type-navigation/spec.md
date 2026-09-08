## MODIFIED Requirements

### Requirement: Provide separate recommendation mode destinations

The Recommendation Engine MUST provide clearly labeled destinations for Song Recommendations and Album Recommendations instead of requiring the user to choose the primary mode from a dropdown. Both destinations MUST remain reachable from a shared recommendation entry surface. The Discover Songs and Explore Albums choice cards MUST appear only on that entry surface, never on either dedicated recommendation page. Shared navigation MUST remain available on both dedicated pages.

#### Scenario: User opens the recommendation entry surface
- **WHEN** the user navigates to the Recommendation Engine entry surface
- **THEN** the user sees distinct, clearly labeled choices for Song Recommendations and Album Recommendations

#### Scenario: User opens a recommendation mode
- **WHEN** the user selects either recommendation mode
- **THEN** the system opens that mode's dedicated page with its mode-appropriate session and presentation

### Requirement: Render recommendation modes with appropriate presentation

The song page MUST render candidates as a flat, single-column, scannable list without individual card borders, shadows, or a multi-column grid with a direct destination link when available. The album page MUST retain a richer card-based presentation with album metadata and album-level actions.

#### Scenario: Song batch is displayed
- **WHEN** a song recommendation batch is available
- **THEN** each candidate is displayed as a list item containing its song identity and an actionable direct link when a destination exists

#### Scenario: Album batch is displayed
- **WHEN** an album recommendation batch is available
- **THEN** candidates are displayed using the album-oriented card presentation and album feedback controls

#### Scenario: Song list is viewed on a narrow screen
- **WHEN** a song batch is displayed on a narrow viewport
- **THEN** songs remain in one column and identity text and actions wrap without horizontal overflow

#### Scenario: Dedicated page is opened directly
- **WHEN** the user opens either dedicated recommendation URL or refreshes it
- **THEN** entry choice cards remain hidden and shared navigation remains visible
