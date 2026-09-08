## Why

The current recommendation workflow is album-first: it helps identify a release, but it adds friction when the user only wants to try the specific song that motivated the recommendation. Individual-song recommendations would make the existing discovery engine more immediately actionable while preserving the current album workflow.

Apple Music is the MVP streaming target because the project already integrates Apple Music data and has an established iTunes Search lookup path. A YouTube fallback ensures a useful destination when the recommended recording cannot be matched in Apple Music. Creating playlists would be more convenient, but requires provider authentication and write APIs, so it is deferred until the link-based flow is proven.

## What Changes

- Add an individual-song recommendation mode alongside the existing album recommendation mode.
- Return verified song-level metadata, including title, artist, album, and any available release/runtime context.
- Resolve each recommended song to an Apple Music song URL using strict artist/title matching.
- Fall back to a YouTube search/watch URL when no matching Apple Music song URL is available.
- Persist and expose the selected destination URL and its provider so song recommendations remain actionable in current and retained recommendation responses.
- Render song recommendations as links in the relevant web/API surfaces while preserving plain-text behavior when no destination can be resolved.
- Keep playlist creation, provider account linking, playlist writes, and user-selectable provider preferences out of this change; document them as a follow-up extension.

## Capabilities

### New Capabilities

- `song-recommendations`: individual-song recommendation output and provider-link resolution, including Apple Music primary links and YouTube fallback links.

### Modified Capabilities

- `streaming-links`: extend link resolution and presentation from recommended albums to recommended songs, with provider identification and YouTube fallback behavior.

## Impact

- **Recommendation generation:** add a song-oriented mode that reuses the existing verified discovery and ranking boundaries rather than inventing unverified track names.
- **Streaming resolution:** extend the existing Apple Music lookup seam and add a best-effort YouTube fallback without making external availability a hard dependency.
- **Persistence/API:** add song recommendation fields and destination-provider metadata to recommendation candidate storage and JSON responses, with additive migration behavior for existing databases.
- **Web UI:** add a song recommendation presentation/link target without regressing the existing album Current Batch links.
- **Tests:** cover mode selection, strict provider matching, fallback behavior, persistence, response shape, and graceful degradation.