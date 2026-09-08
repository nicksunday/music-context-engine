## MODIFIED Requirements

### Requirement: Local web recommendation runtime contract

The local web recommendation surface SHALL accept an explicit Individual Song mode, present verified recording titles, resolve Apple Music song links first, and use a YouTube search destination as a best-effort fallback. Apple Music song links SHALL expose one explicit opening action that attempts the macOS Music desktop app when supported and internally falls back to the canonical web URL; Windows and Linux SHALL use the web destination, and mobile behavior is deferred. In addition, for a completed Individual Song batch, it SHALL offer an explicit user-triggered action to create an Apple Music playlist when MusicKit JS authorization is available, and a created playlist SHALL expose the same single opening action with internal fallback. Playlist creation SHALL report its resulting URL/reference or a clear authorization, validation, provider, or partial-success outcome. The system MUST NOT launch an app or create playlists automatically during recommendation generation and SHALL continue to support recommendation generation without playlist authorization.

#### Scenario: Individual Song batch offers playlist action

- **WHEN** an Individual Song recommendation batch is generated
- **THEN** the response presents verified song candidates with their resolved links and exposes a separate playlist-creation action without automatically creating a playlist

#### Scenario: Apple Music song link offers desktop-app access

- **WHEN** an Individual Song candidate has a resolved Apple Music destination
- **THEN** the UI presents one Apple Music opening action whose fallback behavior is handled internally

#### Scenario: Playlist action returns provider result

- **WHEN** the user triggers playlist creation for an authorized, non-empty Individual Song batch
- **THEN** the system returns the Apple Music playlist URL/reference and the count of tracks added, including any omitted or duplicate candidates

#### Scenario: Created playlist offers desktop-app access

- **WHEN** an Apple Music playlist is successfully created
- **THEN** the result presents the canonical playlist web URL and an explicit action to attempt opening it in the macOS Music desktop app
- **THEN** the result presents one Apple Music opening action that attempts the Music desktop app where supported and otherwise opens the canonical web URL

#### Scenario: Existing fallback remains available

- **WHEN** an Individual Song candidate cannot resolve to Apple Music and playlist creation is not requested or cannot be completed
- **THEN** the candidate retains its existing best-effort YouTube fallback destination and the recommendation flow remains usable

#### Scenario: Unsupported desktop-app environment preserves web access

- **WHEN** the browser or operating system cannot handle an Apple Music desktop-app deep link
- **THEN** the user can still open the canonical Apple Music web destination and the recommendation flow remains usable