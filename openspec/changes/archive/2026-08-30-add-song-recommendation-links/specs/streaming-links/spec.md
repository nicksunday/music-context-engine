## ADDED Requirements

### Requirement: Resolve recommended songs with provider-aware links

In addition to album candidates, the streaming-link capability MUST resolve song candidates using the verified artist and song title. Apple Music song results MUST be accepted only when artist and title match the candidate under the project's normalization rules. A fallback destination MAY be supplied by YouTube when Apple Music has no accepted match, and the response MUST identify the provider for every non-empty URL.

#### Scenario: Matching Apple Music song is accepted
- **WHEN** Apple Music returns a song whose artist and title match the verified candidate
- **THEN** the candidate receives that song's Apple Music URL and provider value

#### Scenario: Mismatched Apple Music result is rejected
- **WHEN** Apple Music returns results but none match both artist and song title
- **THEN** the candidate does not receive an Apple Music URL and the resolver may proceed to YouTube fallback

#### Scenario: YouTube fallback destination is assigned
- **WHEN** Apple Music has no accepted match and YouTube provides a destination for the verified artist/title pair
- **THEN** the candidate receives the YouTube destination and provider value

### Requirement: Render song destinations without regressing album links

The web recommendation surface MUST render a song title as a clickable link when a destination URL is present, opening it in a new browser tab with the existing safe external-link attributes. It MUST render song text without a link when no destination exists, and MUST preserve the existing album-title link behavior.

#### Scenario: Song title opens its provider destination
- **WHEN** a song candidate contains a streaming URL
- **THEN** the song title is rendered as a link to that URL in a new tab

#### Scenario: Song without destination remains readable
- **WHEN** a song candidate has no streaming URL
- **THEN** its song title is rendered as plain text without a dead link

#### Scenario: Album link behavior is unchanged
- **WHEN** an album candidate contains an existing Apple Music album URL
- **THEN** the album title remains a link to the album page in a new tab