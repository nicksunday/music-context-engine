## 1. Reproduce and instrument

- [ ] 1.1 Add failing synthetic fixtures for bare neoclassical eligibility, explicit hard performance requirements, multiple songs per album, repeated recording IDs, artist namesakes and heavier positive fit feedback.
- [ ] 1.2 Retain actual planning, collected candidate evidence and stage rejection decisions with bounded payload sizes and actual provider call counts.
- [x] 1.3 Persist successful empty attempts with an inspectable trace ID without replacing the current useful batch; add legacy missing-evidence handling and browser/API coverage.

## 2. Repair identity and evidence

- [x] 2.1 Carry artist and recording provider IDs through discovery, deduplication, selection and trace contracts with backward-compatible decoding.
- [ ] 2.2 Resolve seed identities within the explicit lookup budget, use identity-bound artist searches, and prevent namesake similarity/evidence transfer; test ambiguous and unavailable identity responses.
- [ ] 2.3 Audit query-tag versus candidate-tag provenance and recording versus album scope, including final metadata/link reconciliation; add controlled fixtures for the suspicious identity and tag failure patterns.

## 3. Repair fit qualification

- [x] 3.1 Separate genre intent, preferred qualities and explicit hard constraints in planning and deterministic qualification; remove the bare-neoclassical performance gate.
- [ ] 3.2 Apply affirmative-relevance and uncertainty policy consistently to model selection, backfill, diversity replacements and degraded fallback; ensure explanations do not invent musical evidence.
- [ ] 3.3 Verify relevant fit context preserves positive heavier discoveries and negative energy feedback without inventing clean-vocal exclusivity, global genre bans or artist-wide track properties.

## 4. Expand song retrieval and enforce variety

- [ ] 4.1 Add explicit song-mode discovery options with album-compatible defaults; implement min(8, max(4, ceil(limit/3))) artist-search and min(96, max(48, 5*limit)) merged-record budgets, at most two tag searches, and initial four-artist/one-tag collection while preserving MCP output limits and tag/source fairness.
- [x] 4.2 Replace song album-level deduplication with recording deduplication; permit four ranking-pool entries per artist and two displayed songs per artist by default, with explicit deep-dive handling.
- [ ] 4.3 Expand insufficient song pools using unused artist/tag sources within the limit-derived budgets and two-tag-search ceiling before selection; retain provider deadlines, rate limits and two model calls.
- [ ] 4.4 Verify model under-selection and diversity replacement fill 20 slots when eligible capacity exists; test cancellations, source failures, exclusions, repeat identities and honest shortfalls.

## 5. Validate and prepare listening acceptance

- [ ] 5.1 Run affected Go tests, deterministic evaluation corpora and relevant browser checks; verify album behavior and legacy persistence on a disposable database copy.
- [ ] 5.2 Review shared-code compatibility with the Feeling Lucky proposal and document effective song budgets, uncertainty behavior and trace inspection.
- [ ] 5.3 Rebuild the local binary and record model/application versions and settings for comparable live runs of the four design prompts, three runs each, plus a held-out request. Report yield, unique artists, repeats, empty outcomes, actual calls and raw latency separately.
- [ ] 5.4 Prepare retained batches and a compact listening scorecard for user judgment; record only actual listening verdicts and leave subjective acceptance pending until those judgments exist. Compare fit and yield together and preserve jazz as a regression check.
