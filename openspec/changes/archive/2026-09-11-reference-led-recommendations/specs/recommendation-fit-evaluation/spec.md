## ADDED Requirements

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

