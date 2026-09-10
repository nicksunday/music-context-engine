## Purpose

Let users curate local request-fit feedback into portable examples that can be deliberately shared and reused without exporting their entire personal database.

## ADDED Requirements

### Requirement: Explicit local review before export
The system SHALL provide a local review surface for selecting feedback examples, inspecting every field proposed for export, editing or removing sensitive text and context, and explicitly approving the resulting payload. Raw records MUST remain local. Export MUST include only selected, approved payloads and MUST NOT upload, commit, or push anything. Changes to an approved payload or its source judgment MUST invalidate approval until reviewed again.

#### Scenario: Unselected records remain private
- **WHEN** a user exports two approved examples from a larger feedback history
- **THEN** only those two reviewed payloads are downloaded

#### Scenario: Review edits do not rewrite history
- **WHEN** a user redacts a personal detail from an export draft
- **THEN** the preview and export contain the redacted text while the original local batch remains intact

#### Scenario: Changed judgment invalidates approval
- **WHEN** an approved example's source judgment is changed or cleared
- **THEN** it cannot be exported using the stale approval

### Requirement: Portable versioned examples
Exports SHALL use documented, versioned JSONL containing a portable example identifier, reviewed request and mode, exact reviewed candidate output, fit verdict, optional reasons or correction, available generation provenance, and reviewed evidence. Local database identifiers, filesystem paths, credentials, and unrelated listening or taste history MUST be excluded. Missing or redacted replay inputs MUST be marked explicitly; export MUST NOT claim such examples are fully reproducible.

#### Scenario: Legacy example exported
- **WHEN** an approved legacy example lacks a discovery snapshot
- **THEN** it exports successfully with incomplete provenance and replay eligibility clearly indicated

#### Scenario: Same approved payload exported again
- **WHEN** the same unchanged approved examples are exported again
- **THEN** their portable identifiers and content remain stable for version-control review

### Requirement: Sharing workflow documentation
The project SHALL document how to review examples, export them, add suitable cases to the shared evaluation corpus, and propose app improvements through normal version control. It MUST explain that exporting examples does not train Ollama or automatically improve other installations.

#### Scenario: Contributor follows the sharing guide
- **WHEN** a contributor follows the documented workflow
- **THEN** they can create a reviewable dataset change and run its evaluations without publishing their local database
