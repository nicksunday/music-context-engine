## 1. Database: persist the resolved streaming URL

- [x] 1.1 Add a nullable `streaming_url TEXT` column to `recommendation_candidates` in `ensureRecommendationSchema` (`internal/database/db.go`).
- [x] 1.2 Add an additive ALTER migration (`columnExists` + `ALTER TABLE ... ADD COLUMN`) that back-fills `streaming_url` on pre-existing databases, and wire it into `ensureAlignedSchema`.
- [x] 1.3 Add `StreamingURL string` to `RecommendationCandidateInput` and `RecommendationCandidate` (`internal/database/recommendations.go`).
- [x] 1.4 Include `streaming_url` in the `CreateRecommendationBatch` INSERT (column list + bound value, trimmed) via `normalizeRecommendationCandidate`.
- [x] 1.5 Update `internal/database/recommendations_test.go` and/or `internal/database/db_test.go` so fixtures exercise the new column (persist + migration).

## 2. Client resolver: resolve each album to its Apple Music URL

- [x] 2.1 Add a `StreamingLinkResolver` interface and an `AppleMusicLinker` implementation in `internal/web/server.go` that queries the iTunes Search API and captures `collectionViewUrl` for the matching album.
- [x] 2.2 Add a `collectionViewUrl` field to `iTunesSearchResult` and reuse the existing `database.NormalizeAlbumLookup` + `equivalentAlbumTitle` matching to accept only a real artist/album match.
- [x] 2.3 Wire the resolver into `Server`/`Options`/`NewServer`, and construct `NewAppleMusicLinker()` in `cmd/music-vault/main.go` for the web handler.

## 3. Bind link resolution into batch generation

- [x] 3.1 In `handleRecommendations`, after `verifyCandidates` and before `database.CreateRecommendationBatch`, assign each candidate its `StreamingURL` from the resolver.
- [x] 3.2 Treat lookup errors and missing matches as "no URL" (resolve to an empty `StreamingURL`) so batch generation never fails on streaming lookup issues (graceful degradation per spec).

## 4. Return the URL in the recommendations response

- [x] 4.1 Expose `streaming_url,omitempty` on `RecommendationCandidate` so the `/api/recommendations` batch payload includes each resolved URL and omits it when absent.

## 5. Frontend: render album titles as Apple Music links

- [x] 5.1 Convert `renderBatch` in `internal/web/static/app.js` so a candidate with `streaming_url` renders its album title as an `<a href target="_blank" rel="noopener">` Apple Music link.
- [x] 5.2 Keep the plain-text album title for candidates without a `streaming_url` (degrade gracefully).

## 6. Server tests & validation

- [x] 6.1 `NewServer` with a fake `StreamingLinkResolver` that returns URLs; assert the recommendations response includes `streaming_url` and the row is persisted.
- [x] 6.2 Add a resolver that returns an error / no match; assert the batch still completes and candidates are returned without `streaming_url`.
- [x] 6.3 Run the test suite (`go test ./internal/...` or equivalent) and `git status` clean aside from intended changes.