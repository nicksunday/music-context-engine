## Why

The current recommendation experience asks users to browse or prompt through taste-weighted song and album workflows when they sometimes want a low-effort surprise from the full breadth of their listening history. A dedicated Feeling Lucky page provides a one-click discovery path that reflects the genres they have listened to without reinforcing favorites, ratings, feedback, or the frequency of any particular genre.

## What Changes

- Add a third, clearly labeled Feeling Lucky recommendation mode and dedicated page alongside Song Recommendations and Album Recommendations.
- Let the user choose or accept a requested batch size `N`, then return up to `N` randomly selected playable items from a candidate pool informed by all genres represented in the user's listening history and reasonable genre-adjacent areas inferred from that history.
- Sample genres uniformly so heavily represented genres do not dominate the result, and do not apply personal taste, rating, favorite, feedback, recency, or quality weighting.
- Include newly discovered items as well as items already present in listening history; previously listened-to albums/items are explicitly eligible for Feeling Lucky. Inferred genre areas may expand discovery but must not become taste-weighted recommendations.
- Keep the existing song and album workflows and their session behavior unchanged except for shared navigation to the new mode.
- Provide clear empty/limited-result behavior when the profile has no usable genre data or the candidate source cannot supply enough items.

## Capabilities

### New Capabilities

- `feeling-lucky-recommendations`: Dedicated random recommendation mode that samples uniformly across observed and profile-inferred genre areas without taste weighting and permits previously listened-to items.

### Modified Capabilities

- `recommendation-type-navigation`: Add Feeling Lucky as a third destination in the shared recommendation entry surface and navigation while preserving independent mode sessions.
- `recommendation-discovery`: Define the candidate eligibility, genre sampling, unbiased selection, batch-size, and insufficient-data behavior for Feeling Lucky generation.

## Impact

- **Web UI and routing**: Recommendation Engine landing/navigation, a dedicated Feeling Lucky page, one-click/regenerate controls, batch-size input, loading/empty/error states, and result presentation appropriate for mixed song/album discovery.
- **Recommendation generation**: New mode-aware generation path that derives observed and inferred genre buckets from listening history/profile data and samples without taste or genre-frequency weighting; it may combine local history with external catalog candidates.
- **Persistence/session APIs**: Store and restore Feeling Lucky batches independently from song and album sessions, including the requested count and generated candidate metadata.
- **Data and links**: Reuse normalized genre metadata and existing song/album destination-link resolution; previously listened-to items must not be excluded solely because they were heard before.
- **Testing**: Add coverage for navigation, unbiased/genre-balanced sampling, inferred-genre expansion, repeatable batch shape, previously listened-to eligibility, new-item inclusion, and no-usable-data behavior.