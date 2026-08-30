## MODIFIED Requirements

### Requirement: Browse previous session prompts
The server MUST expose a read endpoint that lists past recommendation sessions in reverse chronological order, including each session's prompt, mood, creation time, candidate count, and reply. The web UI MUST render this list so the user can see what they previously asked for. The rendered session text MUST be user-selectable so the user can highlight and copy a past prompt, reply, or candidate text. Each listed session MUST offer a "restore prompt" affordance that repopulates the prompt input with that session's prompt without generating a new batch.

#### Scenario: Past sessions listed newest first
- **WHEN** the user views the session list and more than one session exists
- **THEN** sessions are ordered from newest to oldest, each showing its prompt, date, candidate count, and reply

#### Scenario: No prior sessions
- **WHEN** the user views the session list and no sessions exist
- **THEN** the list shows an explicit empty state

#### Scenario: Session text is selectable and copyable
- **WHEN** the user selects and copies text from a listed session's prompt or reply
- **THEN** the exact displayed text is copied to the clipboard

#### Scenario: Restoring a prompt into the prompt box
- **WHEN** the user activates the restore prompt affordance on a listed session
- **THEN** that session's prompt is placed into the prompt text box and no new recommendation batch is generated

### Requirement: Load a selected past session's batch
The web UI MUST let the user select a past session from the list and load that session's album batch into the Current Batch surface, with candidates and their ranks intact and feedback buttons operable. When loading a session's batch fails, the web UI MUST still restore that session's prompt into the prompt box and show an explicit error state instead of silently returning no batch.

#### Scenario: Loading an older session
- **WHEN** a user selects a past session
- **THEN** that session's candidates are rendered in the Current Batch surface with their verdict buttons operable

#### Scenario: Session retained across refresh
- **WHEN** a user has generated one or more sessions and reloads the page
- **THEN** all previously persisted sessions remain available in the session list

#### Scenario: Failed load shows an explicit error and restores the prompt
- **WHEN** a user selects a past session and loading its batch fails
- **THEN** the UI shows an explicit error state, does not leave the current batch surface blank, and still populates the prompt box with that session's prompt