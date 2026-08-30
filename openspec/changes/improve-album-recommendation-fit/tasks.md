## 1. Artist Variety Tests

- [x] 1.1 Add a web recommendation test proving a normal album prompt keeps at most one displayed candidate per normalized artist when the verified/model-selected list contains multiple albums by the same artist.
- [x] 1.2 Add a web recommendation test proving explicit catalog, discography, deep-dive, or "multiple albums by this artist" wording permits repeated artists while still preserving duplicate album suppression.
- [x] 1.3 Add a fallback recommendation test proving fallback drafts also apply the default one-artist-per-batch rule.
- [x] 1.4 Add or update MCP discovery coverage to confirm the lower-level verified candidate pool still allows up to two candidates per normalized artist and remains an input pool rather than the final displayed-batch contract.

## 2. Artist Variety Implementation

- [x] 2.1 Add a conservative repeat-artist opt-in detector for recommendation request text and mood/context.
- [x] 2.2 Add a normalized-artist batch de-duplication helper for `RecommendationCandidateInput` lists that keeps the first candidate per artist unless repeat-artist opt-in is active.
- [x] 2.3 Apply the helper to model-selected drafts before returning them from discovery selection.
- [x] 2.4 Apply the helper to fallback discovery drafts before returning fallback candidates.
- [x] 2.5 Apply the helper after final verification, exclusion filtering, and avoid filtering, before limiting and persisting recommendation batches.
- [x] 2.6 Update the discovery selection prompt wording to prefer one album per artist unless the prompt explicitly requests a catalog/deep-dive result.

## 3. Comparison-Aware Tests

- [x] 3.1 Add planning tests proving prompts with named references or comparison language produce comparison anchors, intended traits, false-friend traits, bridge traits, and modifiers.
- [x] 3.2 Add ranking tests proving Rage Against the Machine-style prompts preserve funk metal, rap metal, alternative metal, rhythmic groove, or staccato riff traits over generic heavy-metal overlap.
- [x] 3.3 Add ranking tests proving KNOWER-but-heavier prompts preserve jazz-funk, fusion, electronic, synth, groove, or rhythm-section traits while allowing added weight.
- [x] 3.4 Add ranking tests proving Opeth-but-less-death-metal prompts prefer progressive, melodic, atmospheric, or dynamic-contrast evidence over primarily death-metal evidence.
- [x] 3.5 Add a regression ranking test proving Symphony X / Children of Bodom-style prompts prefer melodic, neoclassical, power, progressive, symphonic, speed, or melodic-death evidence over generic technical-extreme evidence.
- [x] 3.6 Add note-grounding tests proving recommendation notes do not claim unsupported reference-artist similarity, genre traits, instrumentation, credits, or tone qualities.

## 4. Comparison-Aware Implementation

- [x] 4.1 Extend the discovery plan model to carry comparison anchors, intended comparison traits, false-friend traits, bridge traits, and comparative modifiers.
- [x] 4.2 Update the discovery planning prompt to decompose comparison language before choosing MusicBrainz tags.
- [x] 4.3 Update derived fallback tag logic for known regression examples through general trait groups rather than one-off artist-only branches.
- [x] 4.4 Update `promptTraitCoverage` so comparison traits are marked supported only when candidate genre evidence supports the intended dimension.
- [x] 4.5 Update `promptAlignmentScore` so required comparison traits and bridge traits outrank broad genre overlap, while false-friend traits reduce rank when unbalanced by intended traits.
- [x] 4.6 Update the discovery selection prompt to include comparison fields and instruct the model to describe only supported comparison dimensions.
- [x] 4.7 Keep fallback notes generic or bridge-specific when exact comparison support is weak.

## 5. Specs, Formatting, and Verification

- [ ] 5.1 Reconcile `docs/specs/09_recommendation_discovery.md` and the active `migrate-docs-specs` recommendation-discovery delta with the refined displayed-batch diversity and comparison-aware behavior.
- [x] 5.2 Run `gofmt` on changed Go files.
- [x] 5.3 Run focused recommendation and MCP tests that cover this change.
- [x] 5.4 Run `go test ./...`.
- [x] 5.5 Run `openspec validate improve-album-recommendation-fit --strict`.
