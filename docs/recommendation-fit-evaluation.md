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