## Purpose

Defines how the current recommendation batch links recommended albums out to a streaming service page. This change resolves each verified album recommendation to its Apple Music page (via the iTunes Search API) during generation, persists that URL with the candidate, and renders the Album Title in the web UI's Current Batch surface as a clickable link. The capability is isolated so additional streaming platforms can be added later without changing the recommendation engine.

## ADDED Requirements

### Requirement: Resolve recommended albums to Apple Music album URLs
When a recommendation batch is generated, each candidate album SHALL be looked up against the streaming link service using the candidate's artist and album title. For Apple Music, the lookup targets the iTunes Search API (`entity=album`) for the artist and album concatenated as the search term. A result is accepted only when the returned artist and album match the candidate under the project's string-normalization rules (artist exact, album title equivalent). Candidates with no accepted match MUST NOT carry a streaming URL.

#### Scenario: Apple Music album found for a candidate
- **WHEN** the streaming link service returns an album whose artist and title match the candidate
- **THEN** the candidate carries the returned Apple Music album URL

#### Scenario: No matching album found for a candidate
- **WHEN** the streaming link service returns no result whose artist and title match the candidate
- **THEN** the candidate carries no streaming URL

#### Scenario: Streaming URL points at the matching album page
- **WHEN** a candidate resolves to an Apple Music album
- **THEN** its streaming URL points to that album's Apple Music page, not to a search query or a parent artist page

### Requirement: Persist the resolved streaming URL with each candidate
The resolved streaming (Apple Music) URL MUST be persisted on the `recommendation_candidates` record alongside the candidate so that the current batch and retained history retain their links. Candidates that fail to resolve persist with an empty streaming URL.

#### Scenario: Resolved candidate persisted with its URL
- **WHEN** a candidate resolves to an Apple Music album URL
- **THEN** the persisted `recommendation_candidates` row stores that URL

#### Scenario: Unresolved candidate persisted without a URL
- **WHEN** a candidate does not resolve to an Apple Music album URL
- **THEN** its persisted row stores no streaming URL

### Requirement: Expose the streaming URL in the current batch response
The `/api/recommendations` response payload for a generated batch MUST include each candidate's resolved streaming URL as `streaming_url` when present. Candidates without a resolved URL omit the field from their JSON object.

#### Scenario: Batch response carries resolved URLs
- **WHEN** a generated batch contains candidates with resolved Apple Music URLs
- **THEN** each such candidate object in the response includes a `streaming_url` field with the album's Apple Music URL

#### Scenario: Unresolved candidates omit streaming_url
- **WHEN** a candidate in the response has no resolved streaming URL
- **THEN** the candidate object omits the `streaming_url` field

### Requirement: Render album titles as streaming links in the Current Batch
The web UI's Current Batch surface MUST render each candidate's album title as a clickable link opening the candidate's Apple Music URL in a new browser tab whenever the candidate has a `streaming_url`. The title MUST be rendered as plain text, without a link, when the candidate lacks a `steaming_url`.

#### Scenario: Album title links to Apple Music
- **WHEN** the Current Batch contains a candidate with a `streaming_url`
- **THEN** the album title is rendered as a clickable link to that URL that opens in a new tab

#### Scenario: Album title is plain text without a link
- **WHEN** the Current Batch contains a candidate without a `streaming_url`
- **THEN** the album title is rendered as plain text with no link

### Requirement: Graceful degradation when link resolution is unavailable
If the streaming link lookup is unavailable (timeout, non-2xx response) or the service is not configured, batch generation MUST still succeed. Such candidates are persisted and returned without a streaming URL, and the web UI must render them as plain-text album titles.

#### Scenario: Streaming link service unavailable
- **WHEN** generation resolves a candidate but the streaming link lookup fails
- **THEN** the batch still completes and the candidate is returned without a streaming URL

#### Scenario: Missing link provider leaves lookup unperformed
- **WHEN** no streaming link provider is configured
- **THEN** candidates are generated without any streaming lookup, and all carry no streaming URL