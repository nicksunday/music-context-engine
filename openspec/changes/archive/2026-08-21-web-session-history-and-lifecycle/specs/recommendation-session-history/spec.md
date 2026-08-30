## Purpose

Defines how the local web UI persists and reloads recommendation sessions so the Current Batch survives a page refresh, past session prompts are browsable and reloadable, and a new album batch is generated only when the user asks for it.

## ADDED Requirements

### Requirement: Reload the latest batch after a page refresh
The web UI MUST restore the most recently generated recommendation batch on page load. The server MUST expose a read endpoint returning the latest persisted batch, including its candidates with their ranks, and the frontend MUST render that batch (or an explicit empty state) when the page loads, rather than starting with no batch.

#### Scenario: Refresh restores the latest batch
- **WHEN** a user reloads the web page after at least one batch has been generated
- **THEN** the Current Batch surface shows the most recently persisted batch and its candidates

#### Scenario: Refresh with no prior batch
- **WHEN** a user loads the web page and no batch has ever been generated
- **THEN** the Current Batch surface shows an explicit empty state

### Requirement: Generate a batch only on explicit user action
A new recommendation batch MUST be generated only when the user submits the Generate Batch form, never automatically on page load or refresh.

#### Scenario: No auto-generation on refresh
- **WHEN** a user reloads the web page
- **THEN** no recommendation is generated; the previously persisted latest batch (or empty state) is shown

#### Scenario: Batch generated on submit
- **WHEN** a user presses Generate Batch
- **THEN** a new recommendation batch is generated and persisted as the latest batch

### Requirement: Browse previous session prompts
The server MUST expose a read endpoint that lists past recommendation sessions in reverse chronological order, including each session's prompt, mood, creation time, candidate count, and reply. The web UI MUST render this list so the user can see what they previously asked for.

#### Scenario: Past sessions listed newest first
- **WHEN** the user views the session list and more than one session exists
- **THEN** sessions are ordered from newest to oldest, each showing its prompt, date, candidate count, and reply

#### Scenario: No prior sessions
- **WHEN** the user views the session list and no sessions exist
- **THEN** the list shows an explicit empty state

### Requirement: Load a selected past session's batch
The web UI MUST let the user select a past session from the list and load that session's album batch into the Current Batch surface, with candidates and their ranks intact and feedback buttons operable.

#### Scenario: Loading an older session
- **WHEN** a user selects a past session
- **THEN** that session's candidates are rendered in the Current Batch surface with their verdict buttons

#### Scenario: Session retained across refresh
- **WHEN** a user has generated one or more sessions and reloads the page
- **THEN** all previously persisted sessions remain available in the session list

### Requirement: Read endpoints must not mutate state
The endpoints that read the latest batch and the session list MUST be read-only: they must not generate, modify, or delete batches or candidates.

#### Scenario: Loading latest batch is non-destructive
- **WHEN** the latest-batch endpoint is called
- **THEN** no batch or candidate rows are created, modified, or removed

#### Scenario: Listing sessions is non-destructive
- **WHEN** the session-list endpoint is called
- **THEN** no batch or candidate rows are created, modified, or removed
