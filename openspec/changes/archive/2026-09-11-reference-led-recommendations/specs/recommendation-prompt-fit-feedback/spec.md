## MODIFIED Requirements

### Requirement: Independent request-fit judgments
The system SHALL expose “Met my request” and “Missed my request” for each persisted album or song recommendation, with optional reasons and notes. The judgment MUST refer to the candidate and its original batch request. It MUST NOT alter taste feedback, album ratings, global affinity signals, discovery exclusions, or model weights. Current judgments SHALL be eligible as contextual evidence for sufficiently related requests, with their original request and identity preserved.

#### Scenario: Enjoyable but unsuitable recommendation
- **WHEN** a user records positive taste feedback and “Missed my request” for the same candidate
- **THEN** both judgments remain independent, taste feedback retains existing exclusion behavior, and the fit judgment can inform related requests without becoming a global dislike

#### Scenario: Song identity is preserved
- **WHEN** two songs on the same album receive different request-fit judgments
- **THEN** each judgment remains attached to its own candidate and is not generalized to the whole album

#### Scenario: Corrected fit evidence
- **WHEN** a judgment is replaced or cleared
- **THEN** future context retrieval uses only the current judgment and retained generation snapshots remain unchanged

## ADDED Requirements

### Requirement: Contextual reference examples
The web workflow SHALL allow users to provide, revise, and remove positive or negative artist, album, or song examples for a request without requiring a song title for artist examples. Examples MUST preserve request context, polarity, entity scope, and supplied project identity. They MUST NOT create global taste ratings, permanent artist exclusions, or changes to model weights. Generation MUST snapshot the effective examples.

#### Scenario: Positive artist example without song
- **WHEN** a user supplies Luca Turilli or a specifically identified Rhapsody project as a positive example for a Symphony X request
- **THEN** the example can guide that request and relevant later context without requiring a track name or applying to unrelated prompts

#### Scenario: Ambiguous project identity
- **WHEN** an example cannot be resolved confidently to a particular project
- **THEN** the system preserves the supplied text and exposes the unresolved identity instead of silently combining similarly named projects

#### Scenario: Negative example
- **WHEN** a user says an artist misses this request
- **THEN** that example steers the request without blacklisting the artist's entire catalog for unrelated requests

