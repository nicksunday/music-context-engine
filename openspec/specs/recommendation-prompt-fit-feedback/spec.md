# recommendation-prompt-fit-feedback Specification

## Purpose

Capture whether individual recommendations satisfy their original requests independently from personal taste, preserving evidence for later review and evaluation.

## Requirements

### Requirement: Independent request-fit judgments
The system SHALL expose “Met my request” and “Missed my request” for each persisted album or song recommendation, with optional reasons and notes. The judgment MUST refer to the candidate and its original batch request. It MUST NOT alter taste feedback, album ratings, affinity signals, discovery exclusions, or model weights.

#### Scenario: Enjoyable but unsuitable recommendation
- **WHEN** a user records positive taste feedback and “Missed my request” for the same candidate
- **THEN** both judgments are retained independently and only the taste feedback participates in existing taste and exclusion behavior

#### Scenario: Song identity is preserved
- **WHEN** two songs on the same album receive different request-fit judgments
- **THEN** each judgment remains attached to its own candidate

### Requirement: Correctable and persistent feedback
The system SHALL retain one current request-fit judgment per candidate, allow replacing or clearing it, and restore it when the batch is reloaded. Optional reason categories SHALL include wrong genre, wrong energy, violated exclusion, inaccurate explanation, and other. Invalid verdicts, unsupported reasons, or candidate/batch mismatches MUST be rejected without mutation. Save failures MUST remain visible and MUST NOT appear as successful feedback.

#### Scenario: Judgment corrected and reloaded
- **WHEN** a user replaces a missed judgment with a met judgment and reloads the session
- **THEN** only the current met judgment is displayed and counted

#### Scenario: Judgment cleared
- **WHEN** a user clears a saved judgment
- **THEN** the candidate returns to an unrated request-fit state without deleting its taste feedback

#### Scenario: Original request remains authoritative
- **WHEN** a user edits the prompt input after restoring a batch and judges one of its candidates
- **THEN** the judgment references the batch's saved original request rather than the edited input

#### Scenario: Invalid association rejected
- **WHEN** a feedback request names a candidate belonging to a different batch
- **THEN** the system returns a validation error and preserves existing feedback

### Requirement: Generation evidence and legacy compatibility
New batches SHALL retain an immutable, versioned local snapshot of the effective request, mode, model identity and available digest, prompt versions, application version, interpreted plan, bounded discovery candidate evidence, selection and final displayed output, and relevant generation settings and diagnostics. Missing provenance MUST be explicitly identified. Snapshots MUST exclude credentials and unrelated history. Older batches MUST remain judgeable without fabricating missing evidence or rerunning generation.

#### Scenario: Generation configuration changes
- **WHEN** the configured model or application prompts change after a batch is created
- **THEN** feedback review still shows the generation provenance and candidate output captured for that batch

#### Scenario: Legacy batch judged
- **WHEN** a user judges a batch created before snapshots were supported
- **THEN** the judgment is saved with the original persisted request and candidate and explicitly incomplete provenance