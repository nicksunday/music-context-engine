## Context

See proposal.md for motivation. The Go web recommender plans with Ollama, discovers verified candidates, applies filters and ranking, and asks Ollama to select candidates. SQLite stores batches and candidates. Existing recommendation_feedback drives taste ranking and exact-album exclusions, so reusing its verdict field would conflate different signals. Batch and candidate identifiers already provide attachment points for the new channel.

This design is required because capture spans generation, persistence, UI, export, and evaluation, with new data and privacy boundaries.

## Goals / Non-Goals

**Goals:** Keep judgment capture quick; retain useful local diagnostic evidence; produce portable reviewed artifacts; evaluate deterministic application behavior without requiring external services.

**Non-Goals:** Online learning, automatic ranking changes from clicks, RL or fine-tuning, model adapters, automatic publication, a shared feedback service, an LLM judge, and comprehensive live-model benchmarking. Prompt-fit feedback is evidence for reviewed improvements, not a universal artist penalty.

## Decisions

### Separate storage and API

Add recommendation_prompt_fit_feedback keyed uniquely by candidate ID, referencing its batch, with verdict met/missed, optional reason and note, and timestamps. An additive endpoint supports save/update and clear; the server validates the candidate/batch association using persisted records. Clearing removes the current judgment. Keep existing taste queries and the MCP feedback contract untouched. Reusing recommendation_feedback was rejected because all existing consumers interpret those verdicts as taste or exclusion signals.

Render a separately labeled request-fit control group in both candidate modes. Restore saved state with batch reads. Reasons appear optionally after a judgment; saving and error states are explicit. The saved batch supplies request context even when the compose input changes. Candidate-level judgments are the initial scope; a whole-batch verdict would obscure which recommendation failed.

### Immutable generation snapshot

Add a versioned JSON snapshot per new batch, persisted atomically with the batch. Capture effective request/settings, the interpreted plan, discovery pool and metadata already available to the bounded pipeline, filtering/selection diagnostics, selection response and final displayed candidates, model name/digest when available, application revision, prompt template hashes, and generation options. Capture only relevant context already supplied to generation; never perform a new discovery query for logging. Record missing model digest or other unavailable fields explicitly rather than failing solely on unavailable optional provenance. Bound snapshot size according to candidate and response limits; mark any omitted evidence as incomplete.

Store no credentials, local paths, or unrelated history. Relevant compact profile context can remain local when necessary to explain the run, but export excludes it by default. Avoid promising exact model replay: stochastic inference and provider changes mean even complete provenance is diagnostic evidence. Existing batches get no invented snapshots and remain usable.

### Review drafts and portable exports

Provide a local feedback review page linked from recommendations. Selection creates export drafts from an explicit field allowlist. The page previews the complete outgoing record and allows editing/removing request text, notes, corrections, and evidence. Approval binds to a hash of the draft and source feedback revision; changed or cleared judgments invalidate it. Validate this binding again on download to prevent stale exports. Store drafts separately from immutable local history.

JSONL schema v1 uses stable portable IDs, mode, reviewed request/candidate, judgment, reasons, optional correction, generation provenance, reviewed evidence, and completeness/replay markers. Exclude internal IDs and timestamps that serve only local bookkeeping. Preserve stable field and record ordering. Corrections are optional future training material, not automatically generated preferred responses. Download only explicitly selected approved drafts; exporting performs no Git or network operation. An automatic database export was rejected because it would mix personal history with reusable examples.

### Deterministic evaluation before model training

Add music-vault eval-recommendations with corpus and optional baseline paths, human-readable output and a JSON report option. Move only the necessary deterministic planning calibration, constraint, ranking, and diagnostics logic into reusable code so the evaluator calls production behavior. Inject stable ordering/seed for replay without changing interactive variety behavior.

Maintain distinct schema types for reviewed feedback examples and executable evaluation cases. Curating a case adds an evaluation stage, frozen request/plan/candidates and any explicitly needed context, and assertions such as required plan traits, excluded candidate IDs, pairwise rank order, or expected shortfall classification. An adapter validates reviewed records and reports missing prerequisites; it never invents expectations from a binary label. Do not run generated assertions or arbitrary code from corpus files.

Use case content hashes and corpus hashes for comparison. Counts distinguish failed, passed, and ineligible cases; non-comparable cases are listed separately. Exit 0 requires at least one eligible case and no failed assertions; use a nonzero validation status for malformed corpora or zero eligible cases. Reports record application/evaluator versions and state replay limits. Model-produced plans are frozen inputs: this measures calibration and downstream behavior, not whether a newly invoked Ollama would produce a better plan. Future live-model evaluation is a separate extension.

Ship a small synthetic corpus under testdata/recommendation-fit with explicit musical metadata assumptions, including a case where no candidate qualifies. Document splitting development examples from held-out regression cases and manually reviewing subjective fit. Changes to prompts or ranking are normal reviewed code changes; this proposal establishes the loop without claiming automatic improvement or training.

## Risks / Trade-offs

- Binary judgments do not explain cause → Optional reasons, original context, and explicit curated assertions; avoid assigning all failures to the model.
- Personal details can occur in free text → Allowlisted drafts, complete preview, explicit approval, and no automatic upload; redaction remains a human review task.
- Snapshots grow local storage → Bound captured evidence to pipeline limits and avoid duplicating unrelated history.
- Subjective musical labels can be mistaken for facts → Preserve provenance and curated assumptions; report unsupported evidence instead of inventing sonic properties.
- Offline replay cannot validate prompt-template improvements end to end → State that limit in reports and require separate manual model checks for such changes.
- Corpus overfitting → Document held-out cases and compare per-case regressions as well as totals.

## Migration Plan

1. Add feedback, snapshot, and export-draft storage using existing additive migration conventions; verify an existing database still opens and existing taste behavior remains intact.
2. Enable generation capture and independent feedback endpoints/UI; legacy sessions use explicit incomplete-provenance state.
3. Add reviewed export, evaluator, synthetic fixtures, and workflow documentation.
4. Validate with focused database, web, export, and offline evaluation tests, followed by the existing Go suite and OpenSpec validation.

Rollback uses the previous app version while leaving additive tables in place. Do not drop captured judgments or snapshots during rollback. No existing verdict labels or batch data require rewriting.
