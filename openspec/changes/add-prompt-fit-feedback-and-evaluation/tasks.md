## 1. Persistence and generation evidence

- [x] 1.1 Add additive migrations and database types for candidate-scoped fit feedback, versioned batch snapshots, and reviewed export drafts with source revisions and approval hashes.
- [x] 1.2 Implement validated save, replace, clear, and read operations; test candidate/batch mismatches, independent song identities, and unchanged taste/exclusion behavior.
- [x] 1.3 Capture bounded generation provenance and evidence from the existing recommendation pipeline and persist it atomically with new batches; explicitly represent missing fields and legacy snapshots.
- [x] 1.4 Test migration from an existing database, snapshot immutability after configuration changes, and legacy batch feedback without regeneration.

## 2. Request-fit feedback UI and API

- [x] 2.1 Add fit-feedback mutation endpoints and include current fit state in batch reads, with validation and visible error responses.
- [x] 2.2 Add separate met/missed controls, optional reasons and notes, and change/clear actions to album and song candidate rendering.
- [x] 2.3 Restore fit state for current and historical batches and ensure feedback uses the original saved request after compose-field edits.
- [x] 2.4 Verify album/song interactions, refresh and historical restoration, simultaneous taste and fit judgments, and failed-save presentation using focused web tests and a browser check.

## 3. Reviewed portable export
- [x] 3.1 Define and document JSONL example schema v1, stable portable identifiers, field allowlist, completeness markers, and validation of required versus removable fields.
- [x] 3.2 Implement local review listing and draft editing with a complete export preview, explicit approval, and invalidation when draft content or source feedback changes.
- [x] 3.3 Add selected-approved-example download with stable ordering and server-side approval checks; exclude local IDs, paths, credentials, and unrelated history.
- [x] 3.4 Test redaction without source mutation, no unselected records, stale approval rejection, cleared feedback, stable repeated exports, and legacy incomplete examples.

## 4. Offline evaluation

- [x] 4.1 Define executable case and report schemas distinct from raw feedback exports, including stage inputs, explicit assertions, content hashes, eligibility reasons, and version metadata.
- [x] 4.2 Extract the necessary production calibration, filtering, ranking, and diagnostics logic for offline replay with injected stable ordering or seeds.
- [x] 4.3 Implement music-vault eval-recommendations with corpus input, optional baseline comparison, human and JSON reports, and failure/validation exit statuses.
- [x] 4.4 Add synthetic album/song fixtures for taste-versus-fit disagreement, exclusions, comparison intent, missing metadata, and no qualifying discovery candidate.
- [x] 4.5 Test repeatability without network or personal database access, known failing assertions, malformed schemas, ineligible and zero-eligible corpora, and changed-case baseline comparison.

## 5. Documentation and verification

- [x] 5.1 Document collecting feedback, reviewing and exporting examples, authoring explicit evaluation assertions, comparing baselines, and contributing curated cases through GitHub.
- [x] 5.2 Document held-out cases, subjective judgment limits, frozen-plan replay limits, and separate manual checks for model prompt changes; explain that this feature does not train Ollama.
- [x] 5.3 Run the focused tests and existing Go suite; run the starter corpus and demonstrate that a deliberately failing temporary case yields a nonzero status without changing committed fixtures.
- [x] 5.4 Validate the OpenSpec change and review the implemented behavior against all three capability specs.
