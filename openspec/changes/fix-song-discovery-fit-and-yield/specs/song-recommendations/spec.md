## ADDED Requirements

### Requirement: Song identity and evidence scope
Song discovery MUST distinguish recording identity from album identity. Distinct qualifying tracks on one album MUST remain eligible, subject to song artist-variety limits and existing exact album exclusions. Duplicate representations of the same recording MUST occupy one slot. Artist, album and recording evidence MUST retain source and scope through selection and destination resolution.

#### Scenario: Two different tracks share an album
- **WHEN** two distinct recordings from one unexcluded album qualify and the artist cap permits both
- **THEN** album-level deduplication does not discard either solely because of the shared album

#### Scenario: Recording appears on several releases
- **WHEN** multiple records share a verified recording identifier across releases
- **THEN** only one is returned

#### Scenario: Only album genre is known
- **WHEN** genre evidence belongs to the album rather than the recording
- **THEN** the evidence retains album scope and is not represented as verified track performance
