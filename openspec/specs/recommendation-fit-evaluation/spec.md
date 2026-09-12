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

### Requirement: Inspectable local generation trace
The system SHALL retain versioned local stage evidence linked to the existing generation snapshot: effective references and examples, retrieved personal-context records, similarity provenance and provider status, candidate evidence and scope, qualification decisions, model selection, final output, and degraded/shortfall reasons. Absent evidence MUST be explicit. A local inspection surface MUST expose this trace without credentials or unrelated listening history; it MUST NOT automatically upload it.

#### Scenario: Retrieval miss versus selection miss
- **WHEN** a batch misses the user's request
- **THEN** inspection can distinguish absent suitable candidates from rejected or poorly ranked retrieved candidates without attributing subjective correctness to genre rules

#### Scenario: Legacy snapshot
- **WHEN** an older batch lacks stage evidence
- **THEN** inspection identifies unavailable fields without regenerating the batch or fabricating provenance

### Requirement: Quality and latency evaluation protocol
Evaluation SHALL supplement deterministic replay with an explicitly invoked live/manual protocol covering request interpretation, retrieval, entity selection, final user-judged fit, and latency. Reports MUST separate subjective fit, deterministic assertions, and provider/model failures. They SHALL include per-stage and total elapsed time, external/model call counts, effective limits, versions, and sample size; aggregate latency reports SHALL include median and p95. No unmeasured performance or fit improvement MUST be claimed.

#### Scenario: Recurring reference prompts
- **WHEN** the Symphony X song prompt and the album variant including Children of Bodom are evaluated
- **THEN** reports retain exact prompt wording and separate modes, with Luca Turilli/project examples explicitly identified as supplied positive evidence rather than universal ground truth

#### Scenario: Model comparison
- **WHEN** backend models are compared for selection quality
- **THEN** they receive the same frozen candidate evidence, request, context, and selection constraints, while fresh planning evaluations are reported separately

#### Scenario: Live evaluation is not offline replay
- **WHEN** the existing offline evaluation command runs
- **THEN** it remains deterministic and network-free, while live results require explicit invocation and are labeled separately

#### Scenario: Latency regression
- **WHEN** before-and-after runs use comparable requests and settings
- **THEN** the report exposes timing and call-count differences alongside fit and returned-count differences, including timeout and empty outcomes