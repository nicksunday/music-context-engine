## Context

See `proposal.md` for the user-facing motivation. The current pipeline obtains a padded MusicBrainz result set, applies stable upstream ordering, asks Ollama to select candidates, and then applies final diversity filtering. That ordering allows repeated refreshes to converge on the same records, while post-selection filtering can reduce a requested batch without backfilling. Existing exact artist+album exclusions and mode-specific limits remain important correctness boundaries.

## Goals / Non-Goals

**Goals:**

- Preserve prompt-fit ranking while adding controlled exploration among near-ties.
- Make the requested count a target that is filled from the verified pool whenever possible.
- Keep album and song diversity rules distinct and deterministic under test.
- Reuse logged recommendation feedback as a lightweight ranking signal.
- Keep external requests bounded and avoid increasing MusicBrainz rate-limit pressure unnecessarily.

**Non-Goals:**

- No reinforcement-learning model, online policy training, or new ML service.
- No change to album exclusion semantics, feedback verdict meanings, or public JSON contracts.
- No claim that randomness alone improves semantic relevance; relevance remains driven by verified metadata and prompt alignment.

## Decisions

### 1. Use weighted exploration, not unconstrained shuffling

Build a larger candidate pool than the displayed limit, score prompt alignment and feedback first, and randomize only within a bounded score band or through a small jitter term. Use a request-scoped seed so each refresh can differ while tests can inject a fixed seed. This is preferable to `ORDER BY RANDOM()` or a full shuffle because weak genre matches must not leapfrog strong matches.

### 2. Select with a diversity-aware backfill pass

Treat the model's selections as preferences, not the complete result. Walk selected candidates first, then the remaining ranked pool, applying mode-specific identity and artist/album rules until the requested limit is reached. Album mode defaults to one album per normalized artist; song mode allows multiple tracks where needed but penalizes repeated artists/albums and suppresses exact track identities.

### 3. Keep verification and exclusions authoritative

Backfill only from candidates already returned by verified discovery and still surviving avoid filters and exact-album exclusions. Never generate synthetic candidates or use feedback to re-enable excluded albums.

### 4. Add feedback as a small, decayed feature

Summarize recent feedback by normalized artist, album, and genre tags. Resolve tags at read time from the referenced recommendation candidate, falling back to local album metadata for older or manually logged feedback. Positive verdicts contribute a modest boost, negative verdicts contribute a penalty, and weights decay with age. Prompt alignment and explicit constraints dominate these values. This avoids duplicating genre data in the feedback table while creating useful logged data for a later offline learning-to-rank or contextual-bandit evaluation without making production behavior dependent on an unvalidated RL policy.

### 5. Instrument shortfall reasons

Record internal, low-cardinality diagnostics for pool exhaustion, exclusion, verification failure, duplicate suppression, and diversity suppression. Emit stable key/value shortfall logs without logging full prompts or additional personal listening data beyond existing local state; do not add a database table or public response fields.

## Risks / Trade-offs

- [Randomness can make results feel less stable] → Keep strong score ordering intact, use bounded jitter, and preserve refresh/session history.
- [Backfill may choose less attractive tail candidates] → Backfill in prompt-fit order and only after applying exclusions and diversity rules.
- [Feedback can reinforce narrow tastes] → Use small recency-decayed weights and retain exploration among eligible candidates.
- [Larger pools increase external work] → Retain current bounded fetch caps and use the existing padded request rather than unbounded retries.

## Migration Plan

No data migration is required. Deploy the code and tests together; existing batches and feedback remain readable. Rollback is a code-only revert, with no database cleanup required.