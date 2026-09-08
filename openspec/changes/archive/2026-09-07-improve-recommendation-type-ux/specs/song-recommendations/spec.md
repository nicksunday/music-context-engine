## ADDED Requirements

### Requirement: Provide streamlined song feedback actions

The song recommendation page MUST provide Liked, Disliked, and Not Today actions for each visible track. Liked and Disliked MUST preserve the existing durable feedback meaning. Not Today MUST remove the track from the current visible list without recording it as a durable like or dislike.

#### Scenario: User likes a song
- **WHEN** the user activates Liked for a song candidate
- **THEN** the system records positive song feedback and updates the candidate's visible feedback state

#### Scenario: User dislikes a song
- **WHEN** the user activates Disliked for a song candidate
- **THEN** the system records negative song feedback and updates the candidate's visible feedback state

#### Scenario: User defers a song
- **WHEN** the user activates Not Today for a song candidate
- **THEN** the candidate is removed from the current visible list and no durable like or dislike is recorded for that action

### Requirement: Preserve direct song destinations in the list experience

The song page MUST show a direct destination link when one is available and MUST keep a song candidate usable when no destination can be resolved. A missing destination MUST NOT produce a broken or fabricated link.

#### Scenario: Song destination is available
- **WHEN** a song candidate has an accepted Apple Music or YouTube destination
- **THEN** the compact list exposes that destination as the candidate's direct link

#### Scenario: Song destination is unavailable
- **WHEN** a song candidate has no accepted destination
- **THEN** the candidate remains visible with its identity and no misleading link