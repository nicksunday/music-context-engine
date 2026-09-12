# Offline recommendation-fit evaluation

Run the deterministic starter corpus with:

```sh
music-vault eval-recommendations \
  --corpus testdata/recommendation-fit/starter.jsonl \
  --json /tmp/recommendation-fit-report.json
```

The evaluator reads only the supplied JSONL corpus. It does not contact Ollama,
MusicBrainz, Last.fm, or discovery services; it does not open the personal
SQLite database or mutate user state.

## Curating cases

Reviewed feedback is evidence, not an executable assertion. Copy only deliberately
redacted examples into a case and add explicit expectations such as required mode,
candidate presence/absence, pairwise order, plan traits, or expected shortfall.
Keep development cases separate from held-out regression cases so failures are
not tuned away. Synthetic cases should cover album and song modes, exclusions,
comparison intent, taste-versus-fit disagreement, missing metadata, and an empty
qualifying discovery pool.

Reports distinguish passed, failed, and ineligible cases. A missing assertion or
required replay input is ineligible, never an automatic pass. Malformed corpora,
unsupported schema versions, zero eligible cases, and failed assertions return a
nonzero status. Baseline comparison marks changed case content as non-comparable.

## Limits and improvement loop

Frozen-plan replay measures deterministic calibration, filtering, ranking, and
diagnostics. It does not measure fresh model prompt interpretation, live discovery
quality, or subjective sonic correctness inferred from missing metadata. Prompt
template changes require separate manual checks against Ollama and held-out cases.

Use failures to propose normal reviewed code or corpus changes through GitHub. This
feature does not train Ollama, perform reinforcement learning, upload feedback, or
automatically improve other installations.

## Explicit live/manual evaluation

Live evaluation is never performed by the offline command. After an operator has
explicitly run the web workflow and recorded one JSON object per observation, create
an aggregate report with:

```sh
music-vault eval-recommendations-live \
  --observations /path/to/observations.jsonl \
  --json /tmp/recommendation-live-report.json
```

Each observation records the exact request and mode, model/application versions,
requested and returned counts, an optional user-judged `fit_judgment`, empty outcome,
provider failure, per-stage timing, total timing, and call counts. Reports label this
as `live_manual` and include sample count, median, and p95 total latency. Subjective
fit is not combined with deterministic assertions or provider failures.

Before/after observations should share a `settings_hash`; otherwise the comparison
must be treated as non-comparable and no improvement claim is valid. Use `cohort` to
distinguish recurring prompts from held-out prompts. Frozen-pool model comparisons
must additionally share `pool_hash` and `request_hash`; unavailable models are
reported as limitations and are never downloaded implicitly.