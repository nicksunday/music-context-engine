## Context

See proposal.md for the problem. The Go web workflow makes planning and selection calls to Ollama around shared MusicBrainz discovery. Last.fm similarity already exists, but the web path seeds it from four global affinity artists and discards edge scores/provenance. Candidate traits are currently inferred by hard-coded tag rules. Existing fit feedback and immutable snapshots provide foundations; extend these rather than creating a parallel feedback system.

## Goals / Non-Goals

**Goals:** Make the current request determine retrieval, make evidence inspectable, and improve fit without unbounded extra work. Preserve existing catalog verification, exact exclusions, source fairness, link resolution, and retained-session behavior.

**Non-Goals:** See proposal.md. In particular, this is contextual personalization rather than model-weight training. No new per-song enrichment service or automatic audio analysis is assumed.

## Decisions

### 1. Preserve a structured reference and similarity graph

#### Approved request and plan contract

Keep RecommendationRequest.Message, Mood, Avoid, Limit, and Mode backward compatible. Add an optional Examples collection serialized as examples; omission or an empty collection preserves the plain-text workflow. Each user example carries entity scope (artist, album, or song), supplied text, artist, album/song title where applicable, positive/negative polarity, and optional note. Artist examples require only the artist identity, not a title. Reject malformed structured examples through normal request validation. Canonical identity and resolution status are server-owned evidence, not facts accepted from model or client assertions.

Use the existing planning-model call to extract references from Message and relevant Mood/Avoid text into modelDiscoveryPlan.References, serialized as references. Explicit structured examples also enter that call as authoritative user input, but the server retains the originals independently. Extend/reuse the reference contract introduced in task 1.2 rather than creating incompatible parallel identity types. Preserve supplied text, entity scope, artist/title components, polarity, optional canonical name and MusicBrainz ID, resolution status (unresolved, resolved, or ambiguous), and origin (structured example or extracted text). Model output supplies an interpretation, never verified IDs or resolution.

Reconcile explicit examples and extracted references after planning: deduplicate only confidently identical entities, preserve provenance, and let an explicit example override a conflicting model interpretation of the same entity's identity or polarity. Unrelated references remain separate; do not merge projects on fuzzy name similarity. Discovery and subsequent context retrieval consume this effective reference collection. Keep the raw request, extracted plan references, and effective references in the generation snapshot so reconciliation can be inspected.

Resolve against available local metadata and existing bounded provider responses when possible. Preserve an unresolved artist's supplied name for Last.fm lookup; no dedicated unbounded resolution pass is required. Song/album examples seed artist lookup only when their artist is known. Ambiguity stays visible and never silently selects another project. This design requires neither a standalone semantic parser nor an extra model call.

Before planning, retrieve local context using structured examples and conservative exact normalized library-name matches in the message. These matches are provisional retrieval hints, not inferred positive references (a name can appear in an avoidance instruction). After planning, enrich context using the reconciled references and then their retrieved neighbors for selection. This sequencing avoids a circular dependency between extracting references and preparing planning context.

Implement the request field and planning contract in task 2.1; task 3.1 supplies durable example editing/persistence, not a prerequisite for reference extraction. Plain-text reference requests must work before the example editor exists.

Extend the discovery plan with resolved reference entities and user examples. Keep supplied text, canonical identity/MBID when available, polarity, and resolution status. Use direct positive references first, with a maximum of four distinct lookup seeds, up to fifteen neighbors per seed, and four selected discovery artists using the existing fair rounds. Preserve all contributing edges for deduplicated neighbors, including Last.fm match scores; do not treat scores as calibrated probabilities or sum them into certainty. Negative examples steer selection, never seed it.

Use affinity seeds only when the request has no usable positive references. If a named project is unresolved, preserve its name for an exact provider lookup; do not guess a different Rhapsody project. If explicit-reference lookups fail, use tag fallback and record why. Expose similarity configuration status without revealing the API key. No recursive neighbor expansion.

Alternative: more prompt rules for individual bands. Rejected because it perpetuates reference flattening and cannot generalize.

### 2. Keep evidence scope separate from inferred intent

Represent candidate evidence with source, entity scope (artist, album, recording), and evidence kind (catalog identity, similarity, genre proxy, explicit local preference/fit, or inference). Remove artist-specific comparison bundles and tag rules that mark technique as supported. Genre tags remain retrieval and broad compatibility signals. Existing album/recording metadata and local feedback provide the initial evidence; absence is recorded rather than filled with model assertions.

Use one bounded selection call over the catalog pool, returning candidate indexes plus structured fit assessment and evidence references. A related artist contributes possible catalog entries under the existing 48-record merged-pool and diversity bounds. Album and song assessment remain separate even where they share metadata.

Alternative: fetch reviews or audio for every candidate. Deferred because it introduces new sources and latency before the current retrieval problem is measured.

### 3. Define qualification before filling the batch

Selections receive one of supported, plausible, insufficient, or contradicted. Supported requires direct relevant entity evidence, such as contextual positive fit feedback. Plausible requires a current-reference similarity path plus compatible entity metadata and no known contradiction, or relevant explicit user example evidence. Tag-only matches for a specific artist-comparison request remain insufficient. General tag requests may qualify from matching tags when they do not claim finer performance qualities.

The model can propose assessments, but deterministic validation checks that cited evidence exists, has the correct scope, and passes exclusions and explicit constraints. Missing evidence for an explicit hard requirement prevents qualification; softer requests can return plausible candidates with restrained explanations. Return supported/plausible candidates only. This is evidence sufficiency, not proof of sonic correctness. Evaluate thresholds with held-out judgments.

Backfill uses the same assessment policy. On model failure, deterministic fallback qualifies only what existing evidence supports and marks the response degraded. Return an explicit empty batch with diagnostics when nothing qualifies; avoid generic upstream-error wording for a successful search with no fit.

### 4. Retrieve local context by request relevance

Query existing normalized artists, albums, tracks, ratings, favorites, listening summaries, and current fit feedback. Match explicit references and known neighbor identities first, then overlapping contextual examples and request traits. Keep at most 24 evidence records and a bounded text budget, retaining signal type and source identity. Planning uses locally available reference context; selection can receive the bounded neighbor-relevant context after retrieval. Global summaries are secondary. Do not add a model call or external request for context retrieval.

Reuse existing per-candidate fit judgments. Add an optional request examples collection to the web input and a small editor for entity type, artist/title, polarity, and optional note; artist examples require no song. Persist request-bound examples and immutable effective-generation copies. Later retrieval uses current examples/judgments, while old snapshots remain unchanged. This is retrieval-based adaptation, not automatic training.

Alternative: include the entire library or fine-tune immediately. Rejected because neither fixes missing candidates and both obscure why a particular request succeeds.

### 5. Extend snapshots and evaluate stages separately

Add a versioned trace to the existing snapshot infrastructure and a local expandable inspection view/API. Record plan, bounded context, provider status, edges, candidate assessments, model-selected indexes, backfill, final output, and stage timings/call counts. Redact credentials and request URLs containing API keys; preserve only relevant local evidence. Legacy records show missing fields. Explicit reviewed export continues through the existing export workflow.

First capture baseline traces for the exact song prompt and a recorded album variant; identify the album wording as a reconstructed variant unless supplied verbatim. Add Luca Turilli and the resolved project as user-provided positive examples, never assert that Last.fm must return them. Synthetic offline cases test policy; live/manual cases test actual interpretation, retrieval, and user fit. Keep held-out cases outside tuning.

Compare selection models using identical frozen candidate/context inputs; measure planning separately. Report fit rate among judged candidates together with returned count, empty batches, latency, and provider failure rates so abstention cannot masquerade as quality improvement.

### 6. Measure latency now; defer caching

Maintain two model calls per normal generation (plan and selection), existing MusicBrainz rate limiting, one bounded search per selected discovery artist plus tag search, bounded existing reconciliation/link work, and cancellation. Do not add per-candidate model calls, unbounded pagination, or retries. Capture total and stage elapsed time plus actual call counts, including verification and link resolution. Benchmark at least 20 comparable runs per mode when feasible; report sample sizes and cold-provider conditions rather than claiming a stable p95 from a handful of requests.

Implement quality changes first within these limits. Record caching in docs/future-work/recommendation-caching.md; choose cache targets and TTLs only after traces identify repeated expensive work. Existing caches elsewhere in the project need not be removed.

## Risks / Trade-offs

- Sparse metadata may produce fewer results → Track yield and fit together; label plausible evidence honestly and retain empty outcomes.
- Similarity can capture broad audience overlap rather than the requested technique → Use it for retrieval, preserve edge provenance, and evaluate catalog entries separately.
- Personal feedback can overfit recurring examples → Retrieve by context, preserve negative and positive evidence, and evaluate held-out prompts.
- Provider calls remain slow without caching → Keep strict bounds and stage measurements; prioritize the deferred work using observed costs.
- Larger snapshots may grow SQLite → Bound candidate/context payloads and attach them to existing retention/export behavior.

## Migration Plan

1. Capture baseline behavior and timings before changing ranking.
2. Add backward-compatible optional request/response fields and additive SQLite migrations for examples and trace versions. Do not rewrite historical snapshots or ratings.
3. Implement reference retrieval and evidence qualification, then relevant-context and feedback reuse, preserving existing public MCP tool parameters and collection guarantees.
4. Validate offline regressions, UI flows, live quality/yield, and latency. Update documentation to distinguish configured similarity, degraded lookup, and fit uncertainty.
5. Roll back application behavior if necessary while leaving additive data readable; never delete user judgments or examples during rollback. Caching and model training require separate future proposals.
