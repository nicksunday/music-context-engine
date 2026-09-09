# Reviewed recommendation-fit exports

Request-fit judgments are local evidence. Use the recommendation page to mark a
candidate **Met my request** or **Missed my request**, optionally record one of
the supported reasons, and add notes. Taste feedback remains a separate signal.

## JSONL schema v1

Each exported line is a JSON object with `schema_version: 1`, a stable
portable `example_id`, the reviewed request and mode, the exact candidate
output, the fit verdict, and `provenance_complete` / `replay_eligible` markers.
The export allowlist excludes local database IDs, filesystem paths, credentials,
unrelated listening history, and the local prompt-fit field. Legacy batches can
be exported, but their provenance is explicitly incomplete and they are not
claimed to be replayable.

## Review and export

1. Open the local review payload for a selected fit judgment.
2. Edit or redact the draft payload without changing the original batch.
3. Inspect the complete JSON payload and explicitly approve it.
4. Download only the selected approved draft IDs as JSONL.

Approval is bound to both the draft content hash and the source judgment
revision. Changing or clearing a judgment invalidates the old approval. Export
does not upload, commit, or push anything, and does not train Ollama or
automatically improve another installation.

Suitable reviewed lines may be copied deliberately into the shared synthetic
evaluation corpus through a normal GitHub pull request after removing personal
details and adding explicit deterministic assertions.