## MODIFIED Requirements

### Requirement: Keep playlist creation outside the link-based MVP

The song recommendation workflow MUST NOT require streaming-service account authentication, playlist write permissions, or automatic playlist creation. A returned song destination MUST be directly actionable as a provider link. The browser workflow MUST NOT expose Apple Music authorization, playlist naming, creation, or retry controls, initialize playlist authorization, or issue playlist write requests.

#### Scenario: Song recommendations are generated without account authorization
- **WHEN** a user requests song recommendations without linking a streaming-service account
- **THEN** the system can return direct Apple Music or YouTube destinations without attempting playlist creation

#### Scenario: Current or retained song batch is displayed
- **WHEN** a user displays a new or retained song batch
- **THEN** no authorization or playlist controls appear, and direct destination links and Liked, Disliked, and Not Today actions remain usable
