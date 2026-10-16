## Context

See proposal.md for scope. The retained September 12 Symphony X batches contain 11 and 8 discovery candidates; the latter selected five and backfilled three. Last.fm reported available with 15 neighbor edges in each. Their snapshots lack the raw candidate pool and plan, so the exact origin of suspicious metadata cannot be reconstructed. Those observations are evidence of underfill, not proof of any particular upstream identity error.

Current discovery uses four artist searches, a 48-record merged pool, one track per album and two candidates per artist. Qualification uses a lexical “neoclassical” performance trigger. The example editor was removed; existing fit feedback and free-text prompts remain the product inputs. Feeling Lucky is a separate active proposal touching shared discovery code.

## Goals / Non-Goals

**Goals:** Repair independently testable failure paths, preserve uncertainty honestly, and make future listening failures diagnosable. Preserve album defaults and MCP callers that omit a mode.

**Non-Goals:** Model training or replacement, an artist-specific whitelist, a technical-death-metal blacklist, mandatory clean vocals, example-editor restoration, and changes to Feeling Lucky behavior.

## Decisions

### 1. Capture the failure contracts before changing selection

Use synthetic provider responses for namesakes, multiple songs per album, repeated recordings, positive heavier examples and missing performance evidence. Real exported judgments remain subjective evidence; do not copy the personal JSON dump into shared fixtures. Record the user's Obsidious clarification in this plan and use an analogous synthetic case in tests. Do not mutate an existing personal judgment merely to attach this conversation's note.

### 2. Separate intent strength from evidence strength

Represent genre, soft desired qualities and explicit hard requirements in the existing planning call. A bare genre token does not imply a hard instrumental-performance requirement. “Technically proficient” remains an important ranking preference unless explicitly made mandatory; lack of performance annotations is uncertainty, not proof of failure. Plausible eligibility requires affirmative scoped genre/reference/contextual support and no contradiction; catalog identity alone is not musical support. Model-generated assertions cannot manufacture supporting evidence. Apply the same policy to selection, backfill and degraded fallback.

Keep explanations qualified when performance or vocal evidence is unavailable. Relevant positive Obsidious feedback can support a heavier discovery; it does not establish properties of every song by that artist. This avoids both the existing all-or-nothing gate and indiscriminately admitting every catalog result.

### 3. Preserve catalog identity end to end

Carry MusicBrainz artist and recording IDs from provider responses through deduplication, similarity attachment and traces. Prefer provider-supplied artist IDs; otherwise perform one bounded identity resolution per selected discovery artist. Reject conflicting artist IDs and leave unresolved names ambiguous rather than assigning a convenient namesake. Do not copy query tags into candidate metadata. Preserve recording/album tag scope and link matching provenance. Inspect where the Resul Balay and Circus Maximus anomalies could enter with controlled provider fixtures; historical snapshots cannot conclusively locate that defect.

Identity resolution has a separate budget of at most eight lookups for song requests and four for album requests, shared deadline and existing provider rate limiting. No retries or model calls are added for identity resolution. Ambiguous seeds lose their similarity path but independently relevant tag-discovered candidates can still qualify.

### 4. Make song capacity mode-aware

Retain the four-artist/one-tag-search/48-record album policy. For songs use at most min(8, max(4, ceil(limit/3))) artist sources and min(96, max(48, 5*limit)) merged recordings. Begin with four artists and one tag query; if eligible capacity is insufficient, spend the remaining budget on unused neighbors and at most one additional distinct planned genre query. Use at most 100 records per provider search, no pagination, no recursive similarity, and no repeated identical queries. Preserve half-pool tag reservation, source fairness, cancellation and the existing two model calls. Expanded discovery completes before selection.

Keep the external MCP maximum of 50 returned candidates. Pass song mode explicitly through the web-to-discovery contract with backward-compatible album defaults. A song ranking pool may retain four candidates per artist, including different songs on one album; displayed batches allow two per artist unless the request explicitly asks for a deep dive. Deduplicate by recording ID when available, using normalized artist/album/track identity for legacy records. Preserve exact album exclusions.

Prefer one song per artist before seconds among comparably fitting candidates. Do not elevate a weak candidate solely for variety. Cross-batch artist novelty is not a hard exclusion; that would rapidly exhaust a narrow request. A full batch is guaranteed only when enough qualifying candidates under these limits exist.

### 5. Make outcomes inspectable and acceptance honest

Retain the actual plan, bounded collected candidates, IDs, evidence, rejection reasons and selected indexes. Instrument actual provider operations rather than treating one orchestration call as one external request. Persist successful empty attempts with a trace ID and keep them inspectable without replacing a useful nonempty current batch. This may use an additive attempt record linked to existing snapshots; preserve historical reads and label absent fields explicitly.

Automated acceptance covers 20 eligible songs filling 20 slots, default artist caps, identity collisions, deduplication, genre-only eligibility, hard constraints, all fallback paths and provider budgets. Live acceptance uses the unchanged Symphony X request, “Neoclassical metal,” “Neoclassical metal with a focus on musicianship,” and the bass-focused modern jazz request at limit 20 with fixed model/settings. Record three runs per prompt initially, sample size and raw latencies; do not claim a stable p95 from that sample. User listening judgments determine sonic fit. Report hits among judged tracks, total yield, unique artists, repeats, empty outcomes and latency separately. Treat eight of ten judged tracks fitting per prompt as a provisional listening target, not a user-approved guarantee or an automated assertion. Preserve the jazz prompt as a regression check and include a fresh held-out request.

## Risks / Trade-offs

- Broader retrieval increases latency → strict per-mode provider budgets, deadline reuse, actual call counts, and before/after timing reports.
- Permitting plausible candidates can lower precision → affirmative relevance required, explicit hard constraints retained, restrained notes, and listening evaluation.
- Artist IDs are sometimes unavailable → explicit ambiguity and independent tag relevance, never silent identity substitution.
- Global genre exclusions would suppress good discoveries → contextual positive/negative feedback with entity scope; no new global bans.
- Shared discovery changes can affect albums or Feeling Lucky → mode defaults, dedicated regression tests, and review against the other proposal before integration.

## Migration Plan

Implement regression fixtures and trace capture first, then identity/qualification changes, then song capacity and diversity. Use additive nullable identity/trace fields or versioned JSON as appropriate. Check legacy reads against a disposable database copy. Run focused Go and offline evaluation suites plus browser tests for empty-trace and feedback behavior. Rebuild the local binary before live comparison and record its version and settings. Roll back application behavior without deleting feedback, rewriting old batches or reversing additive data changes.
