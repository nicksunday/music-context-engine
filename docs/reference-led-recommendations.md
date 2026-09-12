# Reference-led recommendations

## Last.fm setup and status

Set `LASTFM_API_KEY` in the local process environment before starting the web or
MCP service. The recommendation workflow makes at most four direct seed lookups,
with up to fifteen neighbors per seed. If the key is absent, the provider status is
`unconfigured`; if lookups return no usable neighbors it is `no_results`. Both cases
fall back to bounded prompt-tag discovery. API keys are never returned in responses,
snapshots, traces, or diagnostics.

Similarity scores are provider match values, not calibrated probabilities. A
deduplicated neighbor may retain multiple edges when several current references
contribute it. Negative examples never seed discovery. Historical affinity is used
as a seed only when no usable positive current reference exists.

## Request-specific learning

The web request accepts optional artist, album, and song examples. Artist examples do
not require a song. Each example has request scope, polarity, supplied identity, and
optional notes. Examples are stored additively in SQLite and revisions/clears affect
future retrieval only; immutable generation snapshots retain the effective examples
used for that batch. Examples do not create global ratings, artist blacklists, or
model updates.

Taste feedback and request-fit feedback are independent. Listening activity is
reported as activity and is never interpreted as a fit judgment. Corrected or cleared
fit judgments are used only in future contextual retrieval; historical snapshots and
global ratings remain unchanged.

## Evidence limitations

MusicBrainz identity and genre/tag metadata establish catalog identity and broad
compatibility. They do not verify specific instrumentation, technique, rhythm, or
arrangement. Performance claims such as virtuosic lead playing require scoped
evidence; otherwise the candidate remains uncertain or insufficient for a specific
performance request. Album evidence is not silently promoted to song evidence.

When evidence is insufficient, the workflow may return a smaller batch or an empty
successful outcome with a machine-readable shortfall reason. Model timeout and
malformed JSON outcomes are separately labeled as degraded selection with qualified
fallback candidates where available.

## Local trace inspection

Inspect a retained batch without regenerating it or uploading data:

```sh
curl 'http://127.0.0.1:8787/api/trace?id=<batch-id>'
```

The trace includes effective references/examples, bounded local context, provider
status, similarity edges, candidate evidence, qualification, selection, output, and
stage metrics when available. Legacy batches explicitly report missing provenance.

## Evaluation

The deterministic evaluator is network-free:

```sh
music-vault eval-recommendations --corpus testdata/recommendation-fit/starter.jsonl
music-vault eval-recommendations --corpus testdata/recommendation-fit/reference-led.jsonl
```

Live/manual evaluation is explicitly invoked from recorded observations:

```sh
music-vault eval-recommendations-live \
  --observations /path/to/observations.jsonl \
  --json /tmp/recommendation-live-report.json
```

Live reports separate subjective fit, returned yield, empty outcomes, provider
failures, per-stage/total latency, and call counts. Before/after observations must
share a settings hash; recurring and held-out requests should be labeled separately.
Frozen-pool model comparisons must use identical pool/request hashes and record
unavailable models instead of downloading them implicitly.