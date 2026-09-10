# recommendation-fit-evaluation Specification

## Purpose

Provide repeatable offline checks of recommendation prompt handling and candidate selection using curated evidence and explicit expectations, supporting measurable shared improvements.

## Requirements

### Requirement: Offline curated evaluation
The system SHALL provide an evaluation command accepting a versioned curated corpus with frozen inputs and explicit expected constraints, plan properties, or relative candidate ordering. It MUST exercise the applicable application logic without contacting Ollama or discovery services, reading personal databases, or mutating user state. Fixed ordering or seeds MUST make repeated runs reproducible. Exported binary judgments alone MUST NOT be treated as complete evaluation assertions.

#### Scenario: Repeatable offline run
- **WHEN** the same corpus is evaluated twice against the same implementation
- **THEN** per-case outcomes and aggregate counts are identical without network access

#### Scenario: Example lacks assertions or replay inputs
- **WHEN** a reviewed example has no explicit expectations or lacks inputs needed for its requested evaluation stage
- **THEN** it is reported as ineligible with a reason rather than counted as passing

### Requirement: Honest stage-specific reports and comparison
Reports SHALL include corpus identity, application and evaluator versions, per-case assertion results, and pass, fail, and ineligible counts by stage. A baseline comparison SHALL identify improvements and regressions for matching cases and mark incompatible or changed cases as non-comparable. Failures SHALL produce a nonzero command exit status; malformed or unsupported corpus formats SHALL produce explicit errors. The report MUST state that frozen-plan replay does not measure fresh model interpretation or live discovery quality and MUST NOT infer subjective sonic correctness from missing metadata.

#### Scenario: Ranking regression detected
- **WHEN** a change moves a known constraint-violating candidate ahead of a qualifying candidate in a curated ordering case
- **THEN** the report identifies the failed assertion and baseline regression and the command exits nonzero

#### Scenario: Corpus case changed
- **WHEN** a baseline case has different inputs or expectations from the current case with the same identifier
- **THEN** the comparison marks that case non-comparable instead of claiming an improvement

### Requirement: Shared starter corpus and improvement loop
The repository SHALL include synthetic or explicitly reviewed examples covering album and song modes, taste-versus-fit disagreement, exclusions, comparison intent, missing evidence, and missing suitable discovery candidates. Documentation SHALL describe converting failure reasons into explicit assertions, changing prompts or ranking through review, and evaluating changes against cases not used to tune them. Subjective judgments and future live-model evaluations MUST be distinguished from deterministic regression results.

#### Scenario: Discovery pool cannot meet the request
- **WHEN** a curated case contains no qualifying candidate
- **THEN** its evaluation checks the configured shortfall or constraint behavior and does not attribute the missing candidate to model selection quality