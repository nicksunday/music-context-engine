## Context

The current recommendation pipeline verifies MusicBrainz-backed album candidates, persists recommendation batches in SQLite, and exposes them through the local web API/UI. The existing `streaming-links` capability resolves Apple Music album URLs through the iTunes Search API and stores a generic `streaming_url`. See proposal.md for motivation and the capability deltas for externally visible behavior.

## Goals / Non-Goals

**Goals:**
- Add a song-oriented recommendation mode using verified recording metadata.
- Reuse the existing recommendation exclusion, ranking, batch persistence, and feedback boundaries where they remain applicable.
- Resolve Apple Music song links first and provide a best-effort YouTube fallback with explicit provider metadata.
- Preserve the existing album recommendation and album-link contracts.
- Make provider resolution injectable and testable, with no external provider failure blocking generation.

**Non-Goals:**
- No automatic playlist creation.
- No streaming-account OAuth, account linking, playlist write permissions, or provider preference UI.
- No replacement of the existing album recommendation workflow.
- No acceptance of loosely matched or model-invented song identities.

## Decisions

### D1. Add an explicit recommendation mode rather than a separate engine

Represent album versus song output as a request/response mode in the existing recommendation flow. Song mode uses the same verified candidate boundary and batch lifecycle, but ranks and displays recording-level candidates. This avoids duplicating prompt handling, exclusions, feedback persistence, and API lifecycle. A separate service was rejected because it would create divergent recommendation semantics and duplicate safeguards.

### D2. Extend candidate identity with song fields and provider metadata

Add song title and destination-provider fields to the candidate model and persist them with additive schema migration. Keep the existing album and generic `streaming_url` fields compatible; use the provider field to distinguish Apple Music from YouTube rather than encoding provider assumptions in the URL column. Existing rows and album batches must deserialize with empty song/provider values.

### D3. Use a resolver interface with ordered providers

Introduce a small streaming destination resolver seam that accepts verified artist/title input and returns an optional URL plus provider. The production chain tries Apple Music first using strict normalized artist/title matching, then YouTube fallback. Test doubles can exercise matching and failure behavior without network calls. Embedding lookups in the recommendation engine was rejected because it couples ranking to provider availability and makes graceful degradation harder to verify.

### D4. Prefer deterministic YouTube search destinations for MVP fallback

When no accepted Apple Music song exists, create a URL-safe YouTube search destination from the verified artist and title, or use a validated direct result if the implementation already has one. The fallback must remain labeled YouTube and must never imply an exact recording match when only a search destination is available. A full YouTube metadata integration is deferred because it would add credentials, quota, and matching complexity without being necessary for an actionable MVP.

### D5. Render provider links from structured fields

The API returns URL and provider separately, and the UI renders the verified song title as the link label. It uses the existing new-tab and `rel="noopener"` safety pattern and falls back to plain text when the URL is empty. Album rendering remains on its current path.

## Risks / Trade-offs

- [YouTube search fallback may not point directly to the intended recording] → label it as YouTube, derive it only from verified metadata, and prefer exact Apple Music matches.
- [Song-level candidates may repeat tracks or artists] → apply explicit song identity deduplication and retain the existing diversity rules where compatible with song mode.
- [Provider lookups add latency] → use bounded timeouts, best-effort resolution, and injectable providers; do not block batch persistence on a lookup.
- [Existing SQLite databases lack new song/provider columns] → use additive migrations and empty defaults for historical rows.
- [A generic URL field can obscure provider semantics] → persist a constrained provider value alongside the URL and omit both when unresolved.
- [Album UI/API regressions] → retain album fields and add focused compatibility tests for existing album batches and links.

## Migration Plan

1. Add additive candidate columns and read/write compatibility for existing databases.
2. Ship song-mode request/response handling and verification before enabling provider links in the UI.
3. Add the ordered Apple Music/YouTube resolver and best-effort wiring; unresolved candidates remain valid.
4. Add song rendering while retaining the existing album rendering path.
5. Roll back by disabling song mode and resolver wiring; additive columns may remain harmlessly in existing databases.

## Open Questions

- Whether a later playlist-creation change should support Apple Music only first or introduce a provider abstraction with OAuth from the outset. This does not affect the direct-link MVP contract.