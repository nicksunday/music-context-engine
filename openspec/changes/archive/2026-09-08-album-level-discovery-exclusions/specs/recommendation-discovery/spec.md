## Purpose

Defines album-first discovery exclusion semantics so recommendations can include unrated albums by known artists while still preventing exact album repeats.

## ADDED Requirements

### Requirement: Album-level discovery exclusions
The system SHALL exclude verified discovery candidates by exact normalized artist+album pairs, not by artist-only, track-only, or standalone album-title tokens. The exclusion set MUST include albums with a whole-album numeric `user_rating`, albums with durable recommendation-feedback verdicts (`disliked`, `ok`, `good`, `great`, `already_know`), and albums with a `not_for_me_today` verdict recorded on the current local calendar day.

#### Scenario: Rated album is excluded
- **WHEN** a discovery candidate has the same normalized artist+album pair as an album row with a non-empty whole-album rating
- **THEN** the candidate is excluded from discovery results

#### Scenario: Durable recommendation-feedback album is excluded
- **WHEN** a discovery candidate has the same normalized artist+album pair as a recommendation-feedback row with verdict `disliked`, `ok`, `good`, `great`, or `already_know`
- **THEN** the candidate is excluded from discovery results

#### Scenario: Same-day not-today feedback is excluded
- **WHEN** a discovery candidate has the same normalized artist+album pair as a `not_for_me_today` recommendation-feedback row created on the current local calendar day
- **THEN** the candidate is excluded from discovery results

#### Scenario: Older not-today feedback does not exclude
- **WHEN** a discovery candidate has the same normalized artist+album pair as a `not_for_me_today` recommendation-feedback row created before the current local calendar day
- **THEN** that feedback row does not exclude the candidate

#### Scenario: Known artist with unrated album remains eligible
- **WHEN** a discovery candidate's artist exists in the local library but the candidate's normalized artist+album pair is absent from album-rating and recommendation-feedback exclusions
- **THEN** the candidate remains eligible for recommendation

#### Scenario: Track-level history does not exclude an unrated album
- **WHEN** the local library contains listened, liked, or favorited tracks for an album but no whole-album rating, durable recommendation-feedback row, or same-day `not_for_me_today` row for that artist+album pair
- **THEN** that album remains eligible for recommendation

### Requirement: Preserve album-repeat protection without global artist blocking
The system MUST continue to suppress duplicate candidate albums within a single discovery response and MUST keep the existing per-artist diversity limit, while allowing candidates from artists already present in local listening history.

#### Scenario: Duplicate album candidate is suppressed
- **WHEN** a discovery response contains multiple candidate tracks from the same normalized artist+album pair
- **THEN** only one candidate for that album is returned

#### Scenario: Known artist diversity limit still applies
- **WHEN** a discovery response contains more than the allowed number of eligible candidate albums from the same normalized artist
- **THEN** the response includes no more than the allowed number for that artist

#### Scenario: Known artist is not globally blacklisted
- **WHEN** a discovery response contains an eligible album by an artist with local library history
- **THEN** artist familiarity alone does not remove that candidate

### Requirement: Consistent web and MCP filtering
The web recommendation endpoint and MCP verified-discovery tool SHALL apply the same album-level exclusion semantics so candidates accepted by MCP discovery are not later removed solely because their artist or matched starter track is known.

#### Scenario: Web endpoint retains eligible known-artist candidate
- **WHEN** MCP discovery returns an unrated, feedback-free candidate by a known artist
- **THEN** the web recommendation endpoint does not remove that candidate due to artist familiarity

#### Scenario: Web endpoint removes rated album candidate
- **WHEN** MCP discovery or model selection returns a candidate matching a rated artist+album pair
- **THEN** the web recommendation endpoint removes that candidate before persisting or displaying the batch

#### Scenario: Web endpoint retains older not-today candidate
- **WHEN** MCP discovery or model selection returns a candidate whose only feedback exclusion evidence is a `not_for_me_today` row created before the current local calendar day
- **THEN** the web recommendation endpoint does not remove that candidate due to the older `not_for_me_today` row
