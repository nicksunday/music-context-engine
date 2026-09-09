# recommendation-type-navigation Specification

## Purpose

Provides a clear shared entry point and navigation model for distinct song and album recommendation workflows, allowing each mode to use an appropriate page and session context.

## Requirements

### Requirement: Provide separate recommendation mode destinations

The Recommendation Engine MUST provide clearly labeled destinations for Song Recommendations and Album Recommendations instead of requiring the user to choose the primary mode from a dropdown. Both destinations MUST remain reachable from a shared recommendation entry surface. The Discover Songs and Explore Albums choice cards MUST appear only on that entry surface, never on either dedicated recommendation page. Shared navigation MUST remain available on both dedicated pages.

#### Scenario: User opens the recommendation entry surface
- **WHEN** the user navigates to the Recommendation Engine entry surface
- **THEN** the user sees distinct, clearly labeled choices for Song Recommendations and Album Recommendations

#### Scenario: User opens a recommendation mode
- **WHEN** the user selects either recommendation mode
- **THEN** the system opens that mode's dedicated page with its mode-appropriate session and presentation

#### Scenario: Dedicated page is opened directly
- **WHEN** the user opens either dedicated recommendation URL or refreshes it
- **THEN** entry choice cards remain hidden and shared navigation remains visible

### Requirement: Provide shared navigation between recommendation modes

Each dedicated recommendation page MUST provide consistent navigation to the other recommendation mode and to the shared Recommendation Engine entry surface. Switching modes MUST NOT silently merge or replace the current session history of the destination mode.

#### Scenario: Switch from songs to albums
- **WHEN** a user selects Album Recommendations from the shared navigation on a song page
- **THEN** the album page opens and the user's album session context is shown independently of the song session context

#### Scenario: Return to the recommendation entry surface
- **WHEN** a user activates the shared Recommendation Engine navigation
- **THEN** the entry surface opens without generating a recommendation batch or changing either mode's persisted session data

### Requirement: Render recommendation modes with appropriate presentation

The song page MUST render candidates as a flat, single-column, scannable list without individual card borders, shadows, or a multi-column grid with a direct destination link when available. The album page MUST retain a richer card-based presentation with album metadata and album-level actions. On dedicated recommendation pages, the Current Batch MUST remain in the primary workbench flow directly below the prompt/composer rather than being separated by unused page space. The song and album pages MUST each initialize the prompt with distinct default text that describes the corresponding recommendation type.

#### Scenario: Song batch is displayed
- **WHEN** a song recommendation batch is available
- **THEN** each candidate is displayed as a list item containing its song identity and an actionable direct link when a destination exists

#### Scenario: Song batch is displayed near the prompt
- **WHEN** a song recommendation page is opened or a song batch is generated
- **THEN** the song Current Batch appears in the primary content flow directly below the prompt controls, without a large empty vertical region separating the two

#### Scenario: Song page keeps compact presentation
- **WHEN** a song recommendation batch is available
- **THEN** each candidate is displayed as a flat list item containing its song identity, an actionable direct link when a destination exists, and the existing song feedback actions

#### Scenario: Album batch is displayed
- **WHEN** an album recommendation batch is available
- **THEN** candidates are displayed using the album-oriented card presentation and album feedback controls

#### Scenario: Album page retains album presentation
- **WHEN** an album recommendation batch is available
- **THEN** candidates remain displayed using the album-oriented card presentation and album feedback controls, with the batch positioned in the same prompt-to-results workbench flow

#### Scenario: Dedicated pages use different defaults
- **WHEN** the user opens the Song Recommendations page
- **THEN** the prompt is initialized with song-oriented default text
- **WHEN** the user opens the Album Recommendations page
- **THEN** the prompt is initialized with album-oriented default text different from the song default

#### Scenario: Mode-specific defaults do not overwrite user input
- **WHEN** the user edits the prompt or restores a previous session prompt
- **THEN** navigating or rendering the current batch does not replace that user-provided prompt with a mode default

#### Scenario: Song list is viewed on a narrow screen
- **WHEN** a song batch is displayed on a narrow viewport
- **THEN** songs remain in one column and identity text and actions wrap without horizontal overflow