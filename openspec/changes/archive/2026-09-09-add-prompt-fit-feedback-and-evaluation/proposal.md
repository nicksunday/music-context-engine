## Why

Existing recommendation reactions capture taste, but cannot distinguish enjoyable music from recommendations that satisfy the request. Separate prompt-fit judgments will expose failures in interpretation, discovery, ranking, and explanations, and provide reusable evidence for improving the app for everyone.

## What Changes

- Add independent “Met my request” and “Missed my request” controls to album and song candidates, including restored sessions, with optional failure reasons and notes.
- Persist judgments against the original request and exact candidate, with versioned generation context for new batches; keep taste ratings and discovery exclusions independent.
- Provide a local review and export workflow for explicitly selected, edited examples in a portable, versioned JSONL format. Raw feedback remains local; exporting never commits or uploads data.
- Add an offline evaluation command and a small curated fixture corpus to measure prompt interpretation, candidate ordering, and constraint handling against explicit expectations, with comparable baseline reports.
- Document how reviewed examples become shared evaluations and inform reviewed prompt or ranking improvements. Preserve data useful for future training without introducing automatic learning, RL, fine-tuning, or model distribution in this change.

## Capabilities

### New Capabilities

- `recommendation-prompt-fit-feedback`: Independent candidate-level request-fit judgments and immutable generation context.
- `recommendation-feedback-export`: Local review, redaction, and portable export of selected feedback examples.
- `recommendation-fit-evaluation`: Offline regression evaluation and a documented improvement loop using curated cases.

### Modified Capabilities

None. Existing taste-feedback contracts remain in place; the new feedback channel is additive.

## Impact

Additive SQLite migrations and database accessors; web generation, candidate rendering, session restoration, and new prompt-fit/review endpoints; a CLI evaluation command and reusable evaluation package; curated JSONL fixtures and contributor documentation. No new hosted service, training dependency, MCP tool change, or automatic Git operation is required. Legacy batches remain usable with explicitly incomplete generation provenance.
