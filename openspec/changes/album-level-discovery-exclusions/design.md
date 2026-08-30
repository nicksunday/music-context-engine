## Context

The current discovery path builds one flat normalized string set from `tracks`, `albums`, and `recommendation_feedback`, then removes a candidate if its artist, album, starter track, or alias appears in that set. That makes the exclusion system simple but overly broad: a known artist token blocks every future album by that artist, and track-level history can block albums that have never been rated as albums. See `proposal.md` for motivation and the delta spec for required behavior.

## Goals / Non-Goals

**Goals:**
- Make the default recommendation-discovery exclusion policy album-first and exact: normalized artist+album pair, not standalone artist/title tokens.
- Keep numeric-rated albums and durable recommendation-feedback albums from repeating.
- Treat `not_for_me_today` as a same-local-day cooldown rather than a durable album exclusion.
- Allow known-artist unrated albums and albums with only track-level history to remain candidates.
- Keep MCP discovery and the web endpoint aligned so a candidate is not accepted by one filter and rejected by another for artist familiarity.

**Non-Goals:**
- No schema migration or new persisted user setting in this change.
- No scoring boost for albums with liked tracks; this change only changes eligibility.
- No UI toggle for stricter "totally unfamiliar album" discovery.
- No change to per-batch duplicate suppression or the same-artist diversity cap.

## Decisions

**D1. Use a typed album-exclusion set instead of reusing the flat string map.**
Add a discovery-specific exclusion representation keyed by normalized artist+album pairs. The set is populated from albums with `user_rating IS NOT NULL`, durable `recommendation_feedback` rows, and current-local-day `not_for_me_today` feedback rows. Candidate filtering checks the candidate's normalized artist+album pair plus any known alternate album titles against that set.
- *Why:* exact pair matching prevents known artists, common album titles, and matched track names from accidentally suppressing valid albums.
- *Alternative considered:* keep the flat `map[string]bool` and simply stop inserting artist tokens. Rejected because album-title-only and track-title-only collisions would still be possible.

**D2. Track-level listening history remains evidence, not an exclusion source.**
The album-exclusion query does not read `tracks`, even when a track is favorited or disliked. If the album has not been rated as a whole and has no recommendation-feedback row, it remains eligible.
- *Why:* a liked track is useful recommendation context for album discovery. Treating it as a veto prevents the engine from suggesting albums that are plausible next listens.
- *Alternative considered:* exclude albums with any favorited track. Rejected for default behavior because it makes "album I have not rated" stricter than the user's current intent.

**D3. Split recommendation feedback into durable exclusions and same-day cooldowns.**
Feedback verdicts `disliked`, `ok`, `good`, `great`, and `already_know` create durable exact-album exclusions. `not_for_me_today` excludes the exact album only when its feedback timestamp falls on the current local calendar day; older `not_for_me_today` rows do not exclude the album.
- *Why:* the durable verdicts communicate an album-level decision or recognition. `not_for_me_today` is mood/context-dependent and should avoid immediate repetition without permanently hiding the album.
- *Alternative considered:* make every feedback verdict durable. Rejected because it turns a temporary mood mismatch into permanent discovery memory.
- *Alternative considered:* never exclude `not_for_me_today`. Rejected because the same album could reappear immediately after the user just said it is wrong for today.

**D4. Keep old flat exclusions only where still needed outside discovery.**
If existing MCP or diagnostic code still needs the flat `GetExclusionList` shape, keep it for compatibility, but the verified-discovery tool and web recommendation endpoint should use the new album-level exclusion API.
- *Why:* this minimizes blast radius while fixing recommendation eligibility.
- *Alternative considered:* mutate `GetExclusionList` in place to return album-pair semantics. Rejected because a `map[string]bool` cannot represent exact pair semantics clearly.

## Risks / Trade-offs

- [More recommendations from familiar artists] -> Keep the existing same-artist cap so a batch can include known artists without being dominated by one catalog.
- [Albums with one liked local track may be recommended] -> Accept as intended default behavior; a later "strictly unfamiliar" mode can use track-level album history as an opt-in filter.
- [Same-day logic depends on local date boundaries] -> Define `not_for_me_today` cooldowns by the server's local calendar date and cover today/yesterday cases in tests.
- [Spec conflict with the docs migration change] -> Target the same `recommendation-discovery` capability path and reconcile wording during implementation/archive so only album-level exclusions remain normative.

## Migration Plan

1. Add the album-level exclusion API and tests without changing the database schema.
2. Route MCP verified discovery through the new album-exclusion set.
3. Route the web endpoint's post-model exclusion through the same album-exclusion set.
4. Update tests and docs/spec wording that currently describe artist, album-title-only, or track-title exclusions.
5. Rollback by returning MCP and web discovery to the previous flat exclusion helper; no persisted data migration is required.
