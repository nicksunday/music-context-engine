## MODIFIED Requirements

### Requirement: Render recommendation modes with appropriate presentation

The song page MUST render candidates as a flat, single-column, scannable list without individual card borders, shadows, or a multi-column grid with a direct destination link when available. The album page MUST retain a richer card-based presentation with album metadata and album-level actions. On dedicated recommendation pages, the Current Batch MUST remain in the primary workbench flow directly below the prompt/composer rather than being separated by unused page space. The song and album pages MUST each initialize the prompt with distinct default text that describes the corresponding recommendation type.

#### Scenario: Song batch is displayed near the prompt
- **WHEN** a song recommendation page is opened or a song batch is generated
- **THEN** the song Current Batch appears in the primary content flow directly below the prompt controls, without a large empty vertical region separating the two

#### Scenario: Song batch is displayed
- **WHEN** a song recommendation batch is available
- **THEN** each candidate is displayed as a list item containing its song identity and an actionable direct link when a destination exists

#### Scenario: Song page keeps compact presentation
- **WHEN** a song recommendation batch is available
- **THEN** each candidate is displayed as a flat list item containing its song identity, an actionable direct link when a destination exists, and the existing song feedback actions

#### Scenario: Album page retains album presentation
- **WHEN** an album recommendation batch is available
- **THEN** candidates remain displayed using the album-oriented card presentation and album feedback controls, with the batch positioned in the same prompt-to-results workbench flow

#### Scenario: Album batch is displayed
- **WHEN** an album recommendation batch is available
- **THEN** candidates are displayed using the album-oriented card presentation and album-level feedback controls

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