## Why

Individual Song recommendations currently provide individual Apple Music links, but users must manually collect the returned tracks into a playlist. An MVP playlist action would turn a recommendation batch into a usable listening queue with one local workflow, while keeping the existing album mode and fallback link behavior intact.

## What Changes

- Add an Apple Music playlist creation action for completed Individual Song recommendation batches.
- Resolve the batch’s verified song candidates to Apple Music track URLs or identifiers and create a playlist containing successfully resolved tracks in recommendation order.
- Expose playlist creation through the existing local web recommendation experience, including the created playlist URL and clear partial/failure feedback.
- Add an explicit action for Apple Music song and playlist links to attempt opening directly in the macOS Music desktop app, while retaining the canonical Apple Music web URL as a fallback.
- Keep playlist creation limited to Apple Music for the MVP; other providers, album-mode playlists, and automatic playlist creation are out of scope.
- Scope direct desktop-app opening to supported macOS/browser environments; unsupported environments SHALL continue to use the web URL.
- Use MusicKit JS for browser-based Apple Music authorization, with only the required developer configuration supplied by the local server; recommendation generation remains usable without playlist authorization.
- Default new playlist names to a concise timestamp while allowing the user to edit the name before creation.
- Preserve partially created playlists when track insertion is incomplete and provide a manual retry action for failed additions.

## Capabilities

### New Capabilities

- `apple-music-recommendation-playlists`: Create Apple Music playlists from verified Individual Song recommendation batches.

### Modified Capabilities

- `recommendation-discovery`: Extend Individual Song output from link resolution only to optional, user-triggered Apple Music playlist creation.

## Impact

- Affects the local web API/UI and recommendation batch response model.
- Affects Apple Music link presentation by adding an app-opening action that internally falls back to the canonical web URL; Windows and Linux use the web destination, while mobile behavior is deferred.
- Adds an Apple Music integration boundary for MusicKit JS authorization, playlist creation, and track insertion; credentials/tokens must remain outside recommendation content and source control.
- Reuses verified candidate data and existing Apple Music song-link resolution where possible.
- Requires unit/integration coverage for authorization states, empty and partially resolvable batches, ordering, duplicate handling, provider errors, retry behavior, and an opt-in personal-account validation path.