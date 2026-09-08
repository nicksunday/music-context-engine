## Why

The current recommendation surface makes songs and albums share a mode selector and presentation model even though they support different discovery goals and feedback behaviors. This adds friction for song discovery and makes the two session types harder to understand and return to. Separate mode-specific pages with shared recommendation navigation will give each workflow an appropriate layout while preserving a coherent entry point.

## What Changes

- Replace the primary recommendation-mode dropdown with a Recommendation Engine landing/entry surface that clearly links to separate song and album recommendation pages.
- Add shared navigation within the recommendation area so users can switch between Song Recommendations and Album Recommendations without losing the distinction between their sessions.
- Give song recommendations a compact list presentation containing the song title, artist, and direct destination link rather than album-style cards.
- Keep album recommendations in the richer card presentation suited to album metadata and album-level feedback.
- Maintain separate current batches and session history for songs and albums so loading or refreshing one mode does not replace the other mode's session context.
- Streamline song feedback to explicit Liked, Disliked, and Not Today actions; Not Today removes the track from the visible list without treating it as a durable positive or negative preference.
- Preserve existing album feedback semantics and album recommendation compatibility.

## Capabilities

### New Capabilities

- `recommendation-type-navigation`: Shared recommendation landing/navigation and separate song and album session surfaces.

### Modified Capabilities

- `song-recommendations`: Define the song-specific list presentation, direct-link action, and streamlined feedback behavior.
- `recommendation-session-history`: Partition current batches and session history by recommendation type and restore each type independently.

## Impact

- **Web UI and routing**: Recommendation Engine entry point, shared recommendation navigation, separate song/album pages, responsive empty/loading/error states, and mode-specific rendering.
- **Recommendation session APIs**: Preserve or extend session and latest-batch reads with an explicit recommendation type while keeping existing album behavior compatible.
- **Feedback handling and persistence**: Add song-specific Not Today behavior and ensure it removes only the visible candidate while preserving the intended distinction from durable like/dislike feedback.
- **Existing recommendation generation**: Album generation, album cards, album feedback, verified song metadata, and destination-link behavior remain available.
- **Testing**: Add UI/server coverage for navigation, type-isolated session restoration, song list actions, and album regression behavior.