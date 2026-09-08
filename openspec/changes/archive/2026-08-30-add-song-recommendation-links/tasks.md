## 1. Candidate Model and Persistence

- [x] 1.1 Extend recommendation request/response models with an explicit album-versus-song mode and stable song candidate fields.
- [x] 1.2 Add additive SQLite columns for song title and destination provider, preserving existing album rows and streaming URLs.
- [x] 1.3 Update recommendation batch create/read queries and JSON serialization to persist and expose song/provider fields with empty defaults for legacy rows.
- [x] 1.4 Add database tests covering song candidate round trips and compatibility with pre-existing album candidates.

## 2. Verified Song Recommendation Flow

- [x] 2.1 Adapt the verified discovery candidate contract so song mode can carry exact recording title, artist, album, runtime, release year, and genre evidence.
- [x] 2.2 Add song-mode ranking/selection behavior that accepts only candidates backed by verified discovery records.
- [x] 2.3 Add song identity deduplication and preserve applicable artist-diversity behavior for displayed batches.
- [x] 2.4 Wire song mode through the existing recommendation batch lifecycle without changing default album-mode behavior.
- [x] 2.5 Add web/API tests for selecting song mode, returning verified metadata, rejecting unverified drafts, and retaining album compatibility.

## 3. Provider Link Resolution

- [x] 3.1 Define an injectable song destination resolver result containing URL and constrained provider metadata.
- [x] 3.2 Implement strict Apple Music song lookup using artist/title normalization and accept only matching song results.
- [x] 3.3 Implement the YouTube fallback destination from verified artist/title data, clearly labeling search destinations as YouTube.
- [x] 3.4 Compose Apple Music-first and YouTube-fallback resolution with bounded, best-effort failure handling.
- [x] 3.5 Apply resolution to song candidates before persistence and leave unresolved candidates valid without URL/provider fields.
- [x] 3.6 Add resolver tests for exact matches, mismatches, provider ordering, fallback, timeouts/errors, and no-result behavior.

## 4. API and Web Presentation

- [x] 4.1 Include song title, destination URL, and destination provider in current and retained recommendation API responses using stable JSON names.
- [x] 4.2 Render song titles as safe new-tab links when destinations exist and plain text when they do not.
- [x] 4.3 Preserve the existing album-title Apple Music link behavior and add regression coverage for album rendering.
- [x] 4.4 Ensure the UI distinguishes Apple Music and YouTube destinations without implying playlist creation.

## 5. Verification and Documentation

- [x] 5.1 Update the active streaming-links capability documentation with the song-link and provider-fallback delta during implementation review.
- [x] 5.2 Document the song recommendation request/response contract and explicitly record playlist creation as deferred.
- [x] 5.3 Run `gofmt` on changed Go files and the focused database, resolver, MCP, web, and recommendation tests.
- [x] 5.4 Run `go test ./...`.
- [x] 5.5 Run `openspec validate add-song-recommendation-links --strict`.