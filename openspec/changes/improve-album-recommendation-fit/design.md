## Context

The web recommendation flow already splits intent planning, MCP-verified candidate retrieval, local ranking, model selection, external verification, exclusion filtering, and persistence. MCP discovery suppresses duplicate artist+album pairs and caps the raw candidate pool at two albums per normalized artist, while the web layer can still select and persist two albums by the same artist in a small displayed batch. Current prompt planning has useful genre-specific guardrails, but comparison prompts still need a more general structure so "like X", "X but heavier", and "like X but less Y" survive planning and ranking across genres. See `proposal.md` for motivation.

## Goals / Non-Goals

**Goals:**
- Enforce one displayed album per normalized artist for normal album recommendation prompts.
- Keep repeated artists possible for explicit catalog, discography, deep-dive, or named-artist requests.
- Improve prompt planning for comparison language across genres and tones.
- Preserve comparison dimensions through deterministic ranking and model selection.
- Keep final recommendations grounded in MCP-verified candidate metadata and current album-level exclusion behavior.

**Non-Goals:**
- No streaming-service playlist creation.
- No persisted user setting for diversity strictness.
- No database schema migration.
- No durable artist blacklist.
- No MCP tool input/API change for this refinement.
- No external review, credit, or audio-analysis dependency.

## Decisions

**D1. Enforce artist variety in the displayed web batch.**
Add a web-layer batch filter that keeps the first recommendation per normalized artist for normal prompts. Apply it to model-selected drafts, fallback drafts, and the final post-verification candidate list before trimming to the requested limit and persisting the batch.
- *Why:* the user-facing issue is repeated artists in a small recommendation batch, while the lower-level MCP pool benefits from retaining up to two candidates per artist as ranking options.
- *Alternative considered:* reduce MCP discovery to one candidate per artist. Rejected because it would shrink the candidate pool before the web ranker can choose the best-fit album and would require broader MCP behavior changes.

**D2. Detect repeat-artist opt-in from the recommendation prompt.**
Treat repeated artists as allowed only when the prompt explicitly asks for a catalog/deep dive, discography exploration, multiple albums/releases, or more albums by a named artist. The detector should use conservative phrase matching over the current request text and mood/context.
- *Why:* the default experience should maximize artist variety, but catalog exploration is a valid album-discovery workflow.
- *Alternative considered:* expose a UI toggle now. Rejected because the current issue can be solved with intent detection and no new product surface.

**D3. Make prompt wording advisory but server filtering authoritative.**
Update selection prompts to prefer one album per artist, but do not rely on the model to obey that preference. The deterministic filter remains the final guard.
- *Why:* model selection can drift or over-prioritize a pair of albums it sees as strong. A small stable filter is easier to test.
- *Alternative considered:* only change the prompt. Rejected because the failure is structural and should be covered by tests.

**D4. Add a structured comparison plan to discovery planning.**
Extend the model discovery plan shape to capture comparison-specific fields alongside the existing vibe summary, required traits, flexible traits, target vibe, and fallback tags. The new fields should identify:
- reference anchors from the prompt,
- intended comparison traits,
- false-friend traits that look superficially related but should be de-emphasized,
- acceptable bridge traits that can satisfy the request indirectly,
- comparative modifiers such as heavier, lighter, less harsh, more electronic, more melodic, less death metal, or more groove-focused.

Existing callers can continue using the current fields, but the web recommendation flow should preserve these comparison fields through the selection prompt and deterministic scoring.
- *Why:* comparison prompts fail when the reference artist collapses into one broad genre tag. Structured fields let the planner say what the comparison actually means.
- *Alternative considered:* keep adding artist-specific prompt rules. Rejected because it does not generalize across genres and makes every failure case a one-off patch.

**D5. Score candidates against comparison dimensions before broad tag overlap.**
Update deterministic ranking so required comparison traits and acceptable bridge traits score above broad fallback-tag overlap, while false-friend traits subtract from the score when not balanced by intended traits. Keep the current genre-tag evidence approach, but route it through general comparison trait groups rather than one metal-specific branch.
- *Why:* the ranker should prefer the candidate that matches the user's intended comparison, not the candidate that merely shares the largest genre umbrella.
- *Alternative considered:* let Ollama make better comparisons without deterministic scoring changes. Rejected because the system already has a scoring layer and the final behavior should be testable.

**D6. Keep reference-specific knowledge conservative.**
Use curated local comparison heuristics only for common or already-identified prompt patterns, and keep them broad enough to express musical dimensions rather than factual claims about exact arrangements. Example regression fixtures can include Rage Against the Machine, KNOWER, Opeth, Symphony X, and Children of Bodom, but the implementation should represent them through traits such as groove, rap-metal, jazz-funk, electronic fusion, progressive dynamics, melodic/neoclassical virtuosity, and subtractive constraints.
- *Why:* the system can improve comparisons without pretending to have verified audio-analysis knowledge for every candidate.
- *Alternative considered:* fetch external articles or reviews for every comparison. Rejected for this change because it adds sourcing, latency, and reliability complexity.

**D7. Keep notes constrained to supported evidence.**
Continue passing supported and unsupported prompt traits to the selection model, but include comparison dimensions so unsupported reference similarities remain unavailable for note-writing. Fallback notes should stay generic when exact comparison support is weak.
- *Why:* the system should avoid making a broad match sound more reference-aligned than the evidence supports.
- *Alternative considered:* allow freer model prose and rely on user judgment. Rejected because the product goal is trustworthy grounded recommendations.

## Risks / Trade-offs

- [Batch may return fewer than requested albums after artist de-duplication] -> Filter by walking the full ranked list, not by trimming first; only return fewer when the verified pool genuinely lacks enough distinct artists.
- [Prompt-based repeat opt-in may miss unusual wording] -> Use conservative obvious phrases first and leave UI-level controls for a later change if needed.
- [Comparison planning may overfit named examples] -> Model the examples as trait groups and false-friend rules, not as one-off artist branches.
- [MusicBrainz tags can be sparse or inconsistent] -> Prefer deterministic penalties only for clear negative evidence and keep model notes limited to supported traits.
- [False-friend penalties may under-rank valid bridge records] -> Allow bridge traits to offset false-friend penalties when they satisfy the comparison intent.
- [Active docs migration also owns recommendation-discovery wording] -> Reconcile this delta with the active migration spec during implementation/archive, following the prior album-level exclusion change pattern.

## Migration Plan

1. Add focused tests that describe the new diversity and comparison-aware ranking behavior.
2. Add the displayed-batch artist de-duplication helper and repeat-opt-in detector in the web recommendation layer.
3. Apply the helper after model selection, in fallback generation, and after final verification/filtering before persistence.
4. Extend the discovery plan and selection prompt to preserve comparison anchors, intended traits, false friends, bridge traits, and modifiers.
5. Update deterministic ranking and trait coverage to score candidates against comparison dimensions before broad tag overlap.
6. Update recommendation-discovery spec/docs wording to reflect displayed-batch artist variety and comparison-aware recommendations.
7. Roll back by removing the web-layer artist de-duplication calls and ignoring the comparison-specific planning/scoring fields; no data migration is required.
