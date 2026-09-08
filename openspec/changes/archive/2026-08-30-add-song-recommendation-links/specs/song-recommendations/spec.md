## Purpose

Provides an individual-song recommendation workflow that turns verified discovery results into actionable song destinations without replacing the existing album recommendation workflow.

## ADDED Requirements

### Requirement: Support an individual-song recommendation mode

The recommendation surface MUST allow a caller to request song recommendations independently of the existing album recommendation mode. A song-mode response MUST be distinguishable from an album-mode response without changing the behavior of existing album requests.

#### Scenario: Song mode is requested
- **WHEN** a caller requests individual-song recommendations with a valid discovery prompt and limit
- **THEN** the system returns a song recommendation batch whose candidates are song-level recommendations

#### Scenario: Existing album mode remains available
- **WHEN** a caller submits an existing album recommendation request
- **THEN** the system continues to return album-level candidates using the existing contract

### Requirement: Return verified song-level candidate metadata

Each song recommendation MUST be grounded in a verified discovery record and MUST include the exact song title and artist returned by the verification source. When available, the candidate MUST also include album, runtime, release year, and genre evidence. The recommendation layer MUST NOT invent song titles or artists that are absent from verified candidate data.

#### Scenario: Verified song candidate is returned
- **WHEN** the discovery source returns a valid recording and the candidate passes local exclusion and verification rules
- **THEN** the song response contains the verified title and artist, plus available album and supporting metadata

#### Scenario: Unverified song draft is rejected
- **WHEN** ranking or generation produces a song name that cannot be matched to a verified discovery record
- **THEN** the unverified song is omitted from the returned candidate list

### Requirement: Persist and expose song recommendation candidates

Song candidates MUST be persisted with enough information to reconstruct the song recommendation batch, including song title, artist, album when known, recommendation rank/note when present, destination URL when resolved, and destination provider when present. Current and retained recommendation responses MUST expose these fields using stable JSON names and MUST preserve existing album candidate fields.

#### Scenario: Song candidate is persisted and returned
- **WHEN** a song recommendation batch is generated
- **THEN** its song candidates are stored and returned with the same song identity and resolved destination metadata

#### Scenario: Existing album candidate remains compatible
- **WHEN** an existing album recommendation batch is read after the song fields are introduced
- **THEN** the batch remains readable and existing album fields retain their prior meaning

### Requirement: Provide a useful destination or an explicit absence

For each song candidate, the system MUST prefer a strictly matched Apple Music song URL. If no accepted Apple Music match exists, it MUST attempt a YouTube fallback based on the verified artist and song title. A destination provider MUST identify whether the URL is Apple Music or YouTube. If neither destination can be resolved, the candidate MUST remain available without a URL and without a misleading provider value.

#### Scenario: Apple Music song match is preferred
- **WHEN** both Apple Music and YouTube destinations can be resolved for a song
- **THEN** the candidate uses the matching Apple Music URL and identifies Apple Music as its provider

#### Scenario: YouTube fallback is used
- **WHEN** no strictly matching Apple Music song is found but a YouTube destination can be generated or resolved
- **THEN** the candidate carries the YouTube destination and identifies YouTube as its provider

#### Scenario: No destination is available
- **WHEN** neither provider yields an acceptable destination
- **THEN** the candidate is returned without a destination URL and without a provider value

### Requirement: Preserve graceful degradation for provider failures

Streaming destination lookup failures, timeouts, non-success responses, or missing provider configuration MUST NOT cause a valid song recommendation batch to fail. The system MUST return the verified song candidate without the failed destination and the UI/API MUST present it without a broken or fabricated link.

#### Scenario: Apple Music lookup fails and fallback succeeds
- **WHEN** the Apple Music lookup fails but YouTube fallback succeeds
- **THEN** batch generation succeeds and the candidate uses the YouTube destination

#### Scenario: All provider lookups fail
- **WHEN** every configured destination lookup fails
- **THEN** batch generation succeeds and the candidate is returned without a destination link

### Requirement: Keep playlist creation outside the link-based MVP

The song recommendation workflow MUST NOT require streaming-service account authentication, playlist write permissions, or automatic playlist creation. A returned song destination MUST be directly actionable as a provider link.

#### Scenario: Song recommendations are generated without account authorization
- **WHEN** a user requests song recommendations without linking a streaming-service account
- **THEN** the system can return direct Apple Music or YouTube destinations without attempting playlist creation