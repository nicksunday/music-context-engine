## Purpose

This capability lets a user turn a verified Individual Song recommendation batch into an Apple Music playlist without manually copying and searching each recommendation.

## ADDED Requirements

### Requirement: Create an Apple Music playlist from an Individual Song batch

The system SHALL provide an explicitly user-triggered action that creates an Apple Music playlist from the verified song candidates in a completed Individual Song recommendation batch. The playlist SHALL preserve recommendation order, use an editable user-visible name defaulted to a concise timestamp, and return a link or provider reference that the user can open.

#### Scenario: Playlist created from a fully resolvable batch

- **WHEN** the user requests playlist creation for a non-empty Individual Song batch and Apple Music authorization is available
- **THEN** the system creates one Apple Music playlist containing the resolved songs in batch order and returns the playlist URL or provider reference

### Requirement: Provide direct desktop-app access for Apple Music destinations

The system SHALL provide one explicit user-invoked Apple Music opening action for a resolved song or created playlist. On supported macOS desktop environments, the action SHALL attempt to open the Music desktop app; on Windows and Linux, and when the app attempt is unsupported or fails, the action SHALL fall back internally to the canonical Apple Music web URL. Mobile behavior is deferred and SHALL NOT be required by this MVP. The UI SHALL NOT require the user to choose between two visible links.

#### Scenario: Supported macOS environment opens the Music app

- **WHEN** the user explicitly activates an Apple Music song or playlist app-opening action in a supported macOS/browser environment
- **THEN** the system attempts the corresponding Music app deep link and internally falls back to the canonical Apple Music web URL if the app does not open

#### Scenario: Deep link is unsupported or unavailable

- **WHEN** the user activates an Apple Music destination action in an environment that cannot open the Music app deep link
- **THEN** the system opens the canonical Apple Music web URL through the same single action and does not report the destination as unavailable solely because the app deep link failed

#### Scenario: Non-macOS desktop uses the web destination

- **WHEN** the user activates the Apple Music opening action on Windows or Linux
- **THEN** the system opens the canonical Apple Music web URL without attempting a macOS app scheme

#### Scenario: Direct app opening is not automatic

- **WHEN** a recommendation batch is rendered or an Apple Music playlist is created
- **THEN** the system does not automatically launch an app or navigate away without an explicit user action

#### Scenario: User attempts playlist creation for an album batch

- **WHEN** the user requests playlist creation for a batch whose recommendation mode is Album
- **THEN** the system rejects the request without creating a playlist and explains that MVP playlist creation is limited to Individual Song batches

#### Scenario: Empty batch

- **WHEN** the user requests playlist creation for an Individual Song batch with no candidates
- **THEN** the system rejects the request without calling Apple Music and reports that there are no songs to add

### Requirement: Handle incomplete Apple Music resolution

The system SHALL add only candidates that resolve to valid Apple Music tracks, SHALL preserve the relative order of added tracks, and SHALL report the number and identity of candidates that could not be resolved. The system MUST NOT claim full success when one or more requested candidates were omitted.

#### Scenario: Some songs cannot be resolved

- **WHEN** a non-empty Individual Song batch contains both Apple Music-resolvable and unresolvable candidates
- **THEN** the system creates a playlist with the resolvable candidates in their original relative order and reports a partial-success result naming or otherwise identifying the omitted candidates

#### Scenario: No songs can be resolved

- **WHEN** every candidate in an Individual Song batch fails Apple Music resolution
- **THEN** the system does not create an empty playlist and reports that no playlist was created because no candidates resolved

### Requirement: Require safe, explicit authorization

The system SHALL require explicit user initiation and valid MusicKit JS Apple Music authorization before creating a playlist. Recommendation generation and individual song-link display SHALL remain available when playlist authorization is absent. Authorization credentials or tokens SHALL NOT be included in recommendation responses, logs, persisted recommendation content, or source control.

#### Scenario: Authorization is unavailable

- **WHEN** the user requests playlist creation without configured or valid Apple Music authorization
- **THEN** the system does not create a playlist, returns an actionable authorization error, and leaves the recommendation batch unchanged

#### Scenario: Recommendation generated without playlist authorization

- **WHEN** the system generates an Individual Song recommendation while Apple Music playlist authorization is unavailable
- **THEN** the system returns the recommendation and its existing link-resolution/fallback results without attempting playlist creation

### Requirement: Avoid duplicate tracks in one created playlist

The system SHALL include each resolved Apple Music track at most once per playlist request, while retaining the first occurrence’s position in recommendation order and reporting any duplicate candidates that were skipped.

#### Scenario: Batch contains duplicate track references

- **WHEN** multiple candidates resolve to the same Apple Music track in one playlist request
- **THEN** the system adds that track once at the first candidate position and reports later duplicate candidates as skipped

### Requirement: Retry incomplete playlist insertion manually

If playlist creation succeeds but one or more track additions fail, the system SHALL retain the created playlist reference, report the failed candidates, and provide an explicit manual retry action. The system MUST NOT retry failed additions automatically or create a replacement playlist as part of the initial request.

#### Scenario: Some track additions fail after playlist creation

- **WHEN** Apple Music creates the playlist but does not accept every requested track
- **THEN** the system keeps the partial playlist, reports the failed additions, and exposes a manual retry action associated with that playlist

#### Scenario: User manually retries failed additions

- **WHEN** the user explicitly retries failed additions for an existing partial playlist
- **THEN** the system attempts only the outstanding additions and reports the updated result without creating another playlist