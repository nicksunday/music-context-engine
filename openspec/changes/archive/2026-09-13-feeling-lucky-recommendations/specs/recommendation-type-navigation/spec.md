## MODIFIED Requirements

### Requirement: Provide separate recommendation mode destinations

The Recommendation Engine MUST provide clearly labeled destinations for Song Recommendations, Album Recommendations, and Feeling Lucky instead of requiring the user to choose the primary mode from a dropdown. All three destinations MUST remain reachable from a shared recommendation entry surface. The song and album choice cards MUST appear only on that entry surface, never on dedicated recommendation pages. Shared navigation MUST remain available on all dedicated pages.

#### Scenario: User opens the recommendation entry surface
- **WHEN** the user navigates to the Recommendation Engine entry surface
- **THEN** the user sees distinct, clearly labeled choices for Song Recommendations, Album Recommendations, and Feeling Lucky

#### Scenario: User opens Feeling Lucky
- **WHEN** the user selects Feeling Lucky
- **THEN** the system opens the dedicated Feeling Lucky page with its mode-specific controls and session context

#### Scenario: Dedicated Feeling Lucky page is opened directly
- **WHEN** the user opens or refreshes the Feeling Lucky URL directly
- **THEN** entry choice cards remain hidden and shared navigation remains visible

#### Scenario: User opens a recommendation mode
- **WHEN** the user selects Song Recommendations, Album Recommendations, or Feeling Lucky
- **THEN** the system opens that mode's dedicated page with its mode-appropriate session and presentation

#### Scenario: Dedicated page is opened directly
- **WHEN** the user opens either existing dedicated recommendation URL or refreshes it
- **THEN** entry choice cards remain hidden and shared navigation remains visible

### Requirement: Provide shared navigation between recommendation modes

Each dedicated recommendation page MUST provide consistent navigation to the other recommendation modes and to the shared Recommendation Engine entry surface. Switching modes MUST NOT silently merge or replace the current session history of the destination mode, including Feeling Lucky sessions.

#### Scenario: Switch from Feeling Lucky to albums
- **WHEN** a user selects Album Recommendations from the shared navigation on the Feeling Lucky page
- **THEN** the album page opens and the user's album session context is shown independently of the Feeling Lucky session context

#### Scenario: Switch from songs to albums
- **WHEN** a user selects Album Recommendations from the shared navigation on a song page
- **THEN** the album page opens and the user's album session context is shown independently of the song session context

#### Scenario: Return to the recommendation entry surface
- **WHEN** a user activates the shared Recommendation Engine navigation
- **THEN** the entry surface opens without generating a recommendation batch or changing any mode's persisted session data