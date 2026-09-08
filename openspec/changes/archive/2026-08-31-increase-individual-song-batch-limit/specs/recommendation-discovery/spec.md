## ADDED Requirements

### Requirement: Enforce mode-specific recommendation batch limits

The web recommendation API SHALL apply distinct maximum batch limits by recommendation mode. Individual Song requests SHALL accept an effective limit greater than 10, with 20 as the target maximum for this change unless bounded-service or model-context validation establishes a lower safe maximum. Album requests SHALL retain their existing conservative maximum of 10 unless a separately documented constraint review changes it. A request above the applicable maximum SHALL be capped to that maximum rather than causing an unbounded discovery or provider workload.

#### Scenario: Individual Song request exceeds the old limit

- **WHEN** a caller submits a valid Individual Song recommendation request with a requested limit greater than 10 and no greater than the supported song maximum
- **THEN** the system attempts to produce and return a song batch honoring the requested effective limit instead of silently capping it at 10

#### Scenario: Individual Song request exceeds the supported maximum

- **WHEN** a caller submits an Individual Song request above the supported song maximum
- **THEN** the system caps the effective limit at the supported song maximum and keeps all downstream work bounded

#### Scenario: Album limit remains independently enforced

- **WHEN** a caller submits an Album recommendation request with a requested limit above the album maximum
- **THEN** the system caps the effective limit at the album maximum and does not inherit the larger Individual Song maximum

#### Scenario: Existing defaults remain stable

- **WHEN** a caller omits or submits a non-positive recommendation limit
- **THEN** the system uses the existing default batch limit for the selected mode and preserves the existing response contract

### Requirement: Bound expanded song-batch external work

For an Individual Song request, candidate retrieval, ranking, verification, destination lookup, persistence, and presentation SHALL use the effective song limit consistently and SHALL remain bounded by the configured service timeouts, rate limits, model-context constraints, and candidate-fetch expansion policy. Increasing the requested song limit MUST NOT cause an unbounded number of external requests or bypass graceful degradation when a provider cannot satisfy the full batch.

#### Scenario: Larger song batch uses a bounded candidate pool

- **WHEN** an Individual Song request uses an effective limit above 10
- **THEN** discovery receives a bounded fetch limit derived from that effective limit and the configured expansion policy, rather than an unlimited request

#### Scenario: Partial external availability

- **WHEN** discovery, verification, or song-destination lookup cannot supply enough candidates for the effective song limit
- **THEN** the system returns the available verified candidates without fabricated entries or a provider failure that invalidates the entire valid batch

#### Scenario: Album external workload is unchanged

- **WHEN** an Album request is processed after the song maximum is increased
- **THEN** album discovery and downstream work continue to use the album-specific effective limit and existing bounded-service behavior