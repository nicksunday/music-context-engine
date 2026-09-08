## MODIFIED Requirements

### Requirement: Reload the latest batch after a page refresh
The web UI MUST restore the most recently generated recommendation batch for the selected recommendation type on page load. The server MUST expose a read endpoint returning the latest persisted batch for that type, including its candidates with their ranks, and the frontend MUST render that batch (or an explicit empty state) when the page loads, rather than starting with no batch.

#### Scenario: Refresh restores the latest batch for the selected type
- **WHEN** a user reloads the song or album recommendation page after at least one batch has been generated for that type
- **THEN** the page shows the most recently persisted batch for that type and its candidates

#### Scenario: Refresh restores the latest batch
- **WHEN** a user reloads the web page after at least one batch has been generated
- **THEN** the Current Batch surface shows the most recently persisted recommendation batch and its candidates

#### Scenario: Refresh with no prior batch
- **WHEN** a user loads the web page and no batch has ever been generated
- **THEN** the Current Batch surface shows an explicit empty state

#### Scenario: Refresh with no prior batch for the selected type
- **WHEN** a user loads a recommendation page and no batch has ever been generated for that type
- **THEN** that page shows an explicit empty state without displaying the other type's latest batch

### Requirement: Generate a batch only on explicit user action
A new recommendation batch MUST be generated only when the user submits the Generate Batch form for the selected recommendation type, never automatically on page load, refresh, or navigation between recommendation types.

#### Scenario: No auto-generation when changing recommendation type
- **WHEN** a user navigates between song and album recommendation pages
- **THEN** no new recommendation batch is generated

#### Scenario: No auto-generation on refresh
- **WHEN** a user reloads the web page
- **THEN** no recommendation is generated; the previously persisted latest batch for the selected type, or an empty state, is shown

#### Scenario: Batch generated on submit
- **WHEN** a user presses Generate Batch on a recommendation page
- **THEN** a new batch for that page's recommendation type is generated and persisted as that type's latest batch

### Requirement: Browse previous session prompts
The server MUST expose a read endpoint that lists past recommendation sessions for the selected recommendation type in reverse chronological order, including each session's prompt, mood, creation time, candidate count, and reply. The web UI MUST render this list so the user can see what they previously asked for in that mode. The rendered session text MUST be user-selectable so the user can highlight and copy a past prompt, reply, or candidate text. Each listed session MUST offer a "restore prompt" affordance that repopulates the prompt input with that session's prompt without generating a new batch.

#### Scenario: Past sessions are isolated by type
- **WHEN** the user views the session list on a song or album recommendation page
- **THEN** the list contains only sessions for the selected recommendation type, ordered newest first

#### Scenario: Past sessions listed newest first
- **WHEN** the user views the session list and more than one session exists
- **THEN** sessions are ordered from newest to oldest, each showing its prompt, date, candidate count, and reply

#### Scenario: No prior sessions
- **WHEN** the user views the session list and no sessions exist
- **THEN** the list shows an explicit empty state

#### Scenario: Session text is selectable and copyable
- **WHEN** the user selects and copies text from a listed session's prompt or reply
- **THEN** the exact displayed text is copied to the clipboard

#### Scenario: No prior sessions for the selected type
- **WHEN** the user views the session list and no sessions exist for that recommendation type
- **THEN** the list shows an explicit empty state

#### Scenario: Restoring a prompt into the prompt box
- **WHEN** the user activates the restore prompt affordance on a listed session
- **THEN** that session's prompt is placed into the prompt text box and no new recommendation batch is generated