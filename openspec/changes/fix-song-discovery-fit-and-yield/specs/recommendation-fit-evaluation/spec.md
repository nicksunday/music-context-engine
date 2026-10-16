## ADDED Requirements

### Requirement: Inspectable unsuccessful discovery yield
New generation traces MUST retain bounded stage inputs and decisions sufficient to account for raw collection, identity rejection, deduplication, exclusions, qualification, diversity, selection and returned count. Successful empty outcomes MUST receive a retained trace identifier accessible from the response. Missing legacy evidence MUST be labeled explicitly regardless of a legacy completeness flag.

#### Scenario: Empty neoclassical outcome
- **WHEN** a successful request returns no candidates
- **THEN** its retained trace distinguishes no source results, rejected candidates and exhausted work budget without requiring regeneration

### Requirement: Fit and yield regression acceptance
Evaluation MUST cover genre-only neoclassical eligibility, explicit hard constraints, song-level deduplication, artist namesakes, bounded 20-song filling, and heavier positive contextual examples. Live listening evaluation MUST separately report judged fit, yield, artist repetition, empty outcomes and latency for the Symphony X, neoclassical metal, and bass-focused modern jazz requests. No sonic improvement SHALL be claimed from deterministic tests alone.

#### Scenario: Enough eligible recordings exist
- **WHEN** a deterministic source supplies at least 20 distinct qualified songs spanning enough artists within configured bounds
- **THEN** a request for 20 returns 20 without duplicate recordings or exceeding the default artist cap

#### Scenario: User judgments are incomplete
- **WHEN** only selected songs have listening judgments
- **THEN** the report gives the judged sample size and does not count unjudged songs as hits or misses
