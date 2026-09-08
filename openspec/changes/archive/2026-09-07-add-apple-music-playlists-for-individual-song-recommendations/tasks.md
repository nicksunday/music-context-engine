## 1. Provider and authorization boundary

- [x] 1.1 Define the Apple Music playlist-provider interfaces and result types separately from the existing song-link resolver, including authorization status, playlist reference, added count, unresolved candidates, duplicates, and provider errors.
- [x] 1.2 Implement MusicKit JS browser authorization/configuration handling with fail-closed behavior, context-aware requests, and token redaction in logs and errors.
- [x] 1.3 Implement Apple Music track resolution for verified recommendation candidates, reusing compatible existing lookup behavior while preserving per-candidate resolution diagnostics.
- [ ] 1.4 Implement playlist creation and ordered track insertion with bounded provider request batches; retain partial playlists and support explicit retry of outstanding additions without blind retries.
- [x] 1.5 Define a centralized Apple Music destination model that retains the canonical web URL and derives an optional macOS Music app deep link for songs and playlists.

## 2. Recommendation batch action

- [x] 2.1 Add persisted-batch loading for playlist requests and validate that the referenced batch exists, is in Individual Song mode, and contains candidates before contacting Apple Music.
- [x] 2.2 Resolve candidates in persisted recommendation order, deduplicate provider track identifiers with first occurrence winning, and construct structured partial-success results.
- [ ] 2.3 Add local web actions that accept a trusted batch ID and editable playlist name, require explicit initiation, and return success, authorization, validation, provider, empty, partial-success, and manual-retry responses without mutating recommendation records.
- [x] 2.4 Ensure recommendation generation and existing Apple Music/YouTube link fallback paths remain independent of playlist authorization and never create playlists automatically.

## 3. Web UI and user feedback

- [x] 3.1 Add an Individual Song-only “Create Apple Music playlist” control tied to the current persisted batch, with an editable timestamp-based default name.
- [ ] 3.2 Render playlist URL/reference, added-track count, unresolved candidates, duplicate skips, partial-write state, manual retry control, and actionable authorization/provider errors without hiding existing per-song links.
- [x] 3.3 Disable or hide the playlist action when MusicKit authorization is unavailable while keeping recommendation generation and direct-link workflows available.
- [x] 3.4 Add one explicit “Open in Music” action for Apple Music song and playlist destinations, handling app-vs-web fallback internally and avoiding automatic app launches.
- [x] 3.5 Implement macOS app handoff and direct Windows/Linux web behavior; defer mobile behavior without exposing a second visible link.

## 4. Tests and documentation

- [ ] 4.1 Add provider-mock tests for MusicKit authorization failure, full resolution, partial resolution, zero resolution, ordering, duplicate suppression, bounded insertion, provider errors, partial playlist retention, and manual retry.
- [ ] 4.2 Add web-handler tests for album-batch rejection, empty batches, invalid/missing batch IDs, editable names, explicit action behavior, response payloads, manual retry, and preservation of fallback links.
- [ ] 4.3 Add UI/API contract coverage for successful and partial playlist results and verify that credentials never appear in responses, persisted recommendation content, logs, or error strings.
- [ ] 4.4 Update the recommendation and local Apple Music configuration documentation with the MVP scope, authorization setup, playlist action behavior, and rollback/disable procedure.
- [ ] 4.5 Add platform-aware tests for the single song/playlist opening action, macOS app handoff, Windows/Linux web behavior, internal fallback, and deferred mobile behavior.
- [ ] 4.6 Add opt-in documentation and validation instructions for creating a playlist with the user’s own Apple Music account without committing credentials or tokens.