## Context

The web UI's Current Batch surface is rendered from the JSON returned by `POST /api/recommendations`. That handler (in `internal/web/server.go`) runs the Ollama recommender, verifies each draft candidate against MusicBrainz (`MusicBrainzAlbumVerifier.Verify`), persists the batch via `database.CreateRecommendationBatch`, and returns `RecommendationResponse{Batch}`. `internal/web/static/app.js` renders each candidate's album title as plain text inside an `<h3>` (`renderBatch`).

The server already integrates the iTunes Search API (`iTunesAlbumTags` on `MusicBrainzReleaseRadar`) for the release radar's genre tags. That method searches `entity=album` by artist + album and matches results against the target using `database.NormalizeAlbumLookup` (artist exact, `equivalentAlbumTitle` for the title). The same lookup can capture Apple Music's `collectionViewUrl`. See proposal.md — Why for the motivation.

## Goals / Non-Goals

**Goals:**
- Resolve each candidate album in a generated batch to its Apple Music album URL using the existing iTunes Search API.
- Persist the resolved URL per candidate and return it in the `/api/recommendations` payload.
- Render the Album Title in the Current Batch as a clickable Apple Music link (new tab) when a URL exists, plain text otherwise.
- Keep URL resolution isolated so a future streaming-platform-preference feature can add platforms without touching the recommendation engine.

**Non-Goals:**
- No user-facing streaming platform selection/preference in this change (deferred follow-up).
- No new external service or authentication; Apple Music resolution only.
- No changes to the MCP server, ingestion pipeline, or how candidates are otherwise verified/ranked.

## Decisions

**D1. Resolver is a small dedicated provider, implemented for Apple Music.**
Add a `StreamingLinkResolver` interface on `web.Server` (parallel to `CandidateVerifier`/`ReleaseRadarProvider`) with a concrete `AppleMusicLinker`. It runs in `handleRecommendations` after `verifyCandidates` and before `database.CreateRecommendationBatch`, assigning each candidate a `StreamingURL`.
- *Why:* it mirrors the existing verifier/release-radar injection pattern (clean for tests via a fake), keeps the recommendation engine unchanged, and gives a natural seam for additional platforms later.
- *Alternative considered:* piggybacking on `MusicBrainzAlbumVerifier.Verify`. Rejected — the verifier is MusicBrainz-specific, and mixing URL resolution in couples two concerns and complicates its tests.

**D2. Reuse the existing iTunes Search + matching logic rather than a new search stack.**
`AppleMusicLinker` calls `https://itunes.apple.com/search?term=<artist> <album>&media=music&entity=album&limit=10` and accepts the first result whose normalized artist equals the candidate's clean artist and whose title is `equivalentAlbumTitle`-equivalent, reusing `iTunesSearchResult`/`equivalentAlbumTitle` and `database` normalization. It captures `collectionViewUrl` (the Apple Music album page) instead of a genre tag.
- *Why:* zero new services, matching consistent with the release radar, and predictable fallback: on mismatch, timeout, or non-2xx, the candidate keeps no URL (graceful degradation per the spec).

**D3. Persist as a generic `streaming_url` column, not `apple_music_url`.**
Add `streaming_url TEXT` to `recommendation_candidates` (in `ensureRecommendationSchema`) plus an additive ALTER migration for existing databases using the existing `columnExists` + `ALTER TABLE ... ADD COLUMN` pattern (see `ensureAppleMusicPlayActivitySchema`). Expose it on the candidate JSON as `streaming_url,omitempty`.
- *Why `streaming_url`:* the deferred platform-preference feature should store a single current-platform URL per candidate; a platform-prefixed column would force re-migration when a second platform is added.

**D4. Frontend renders the album title as a link only when a URL exists.**
In `renderBatch`, when `candidate.streaming_url` is present, build the album `<h3>` as an `<a href="{{streaming_url}}" target="_blank" rel="noopener">` containing the album title; otherwise keep the existing plain-text `<h3>`.
- *Why:* satisfies the spec's plain-text fallback and avoids dead links for unresolved candidates.

## Risks / Trade-offs

- [iTunes may return an incorrect album for ambiguous artist/title combinations] → reuse strict matching (artist exact + `equivalentAlbumTitle`); on any mismatch the candidate gets no URL rather than a wrong link.
- [iTunes Search availability/network failures during generation] → treat lookup errors as "no URL" (spec: graceful degradation); a single provider is best-effort and does not fail the batch.
- [Existing databases lack the new column] → additive `ALTER TABLE` migration via `columnExists`, mirroring the established schema-migration pass.
- [Storefront/country in `collectionViewUrl` defaults to `us`] → acceptable for MVP; the returned URL still resolves to the Apple Music album. Revisit if a platform-preference feature needs storefront controls.
- [N+1 iTunes lookups at batch generation (~10 candidates)] → acceptable; mirrors per-candidate MusicBrainz verification and uses a bounded per-request timeout; no caching added here.

## Migration Plan

1. Ship the schema change (new `streaming_url` column created by `ensureRecommendationSchema` and back-filled via the ALTER migration for pre-existing DBs) so new and old databases coexist.
2. Deploy the resolver + handler wiring + `app.js` rendering together; the UI degrades to plain text for rows still missing a URL.
3. Rollback: revert rendering/resolver; the extra column is additive and harmless if left in place.

## Open Questions

- Whether a future streaming-platform-preference feature should prefer storefront-aware URLs (e.g., regional search / `?country=`) — decided later; does not affect the spec or this change's approach.