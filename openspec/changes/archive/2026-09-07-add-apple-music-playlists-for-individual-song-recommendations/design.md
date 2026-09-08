## Context

The local Go web server already distinguishes Album and Individual Song recommendation modes, persists verified recommendation batches, and resolves Individual Song candidates to Apple Music links with a YouTube fallback. See `proposal.md` for motivation and the change specs for the externally visible contract. The repository currently has no playlist-write integration or provider authorization flow, so playlist creation must be an optional boundary layered on top of the existing read/link path.

## Goals / Non-Goals

**Goals:**

- Add a provider abstraction that can use MusicKit JS authorization, create a playlist, and add verified tracks.
- Make playlist creation a separate action against an existing persisted Individual Song batch.
- Preserve candidate order, avoid duplicate provider tracks, and return structured partial-success information.
- Keep recommendation generation and existing link fallback independent of playlist authorization.
- Keep developer configuration local, keep MusicKit authorization state out of recommendation persistence, and never expose tokens in logs or API responses.
- Provide one explicit Apple Music opening action with an internal macOS app attempt and web fallback; do not expose two links or require mobile support.

**Non-Goals:**

- Supporting Spotify or other playlist providers.
- Creating playlists for Album mode.
- Automatically creating playlists after every recommendation request.
- Synchronizing, editing, or deleting playlists after creation.
- Replacing the existing Apple Music/YouTube link resolvers.
- Supporting mobile app handoff in this MVP.

## Decisions

### Separate playlist service from link resolution

Introduce a playlist-oriented provider interface distinct from `SongLinkResolver`. Link resolution is unauthenticated/read-oriented and returns URLs; playlist creation needs authenticated write operations and provider track identifiers. Keeping the interfaces separate allows current recommendations to work without credentials and makes the write boundary mockable in tests.

**Alternative considered:** Extend `SongLinkResolver` with playlist methods. Rejected because it conflates read and write permissions and would make an otherwise optional feature a concern for every link resolver.

### Use persisted batch identity as the action input

The web action should accept a recommendation batch ID plus an optional playlist name, load the persisted batch, and verify that its mode is `song`. This avoids trusting client-submitted artist/title data and ensures playlist contents originate from the same verified candidates displayed to the user.

**Alternative considered:** Submit the currently rendered candidate JSON from the browser. Rejected because it permits tampering and can diverge from the durable batch used for feedback/audit.

### Resolve and deduplicate before provider writes

For each candidate in persisted order, resolve an Apple Music track identifier using the candidate’s verified metadata and existing Apple Music lookup behavior where compatible. Track identifiers are deduplicated before the create/add calls; the first candidate wins. Resolution failures and duplicates are retained as structured result metadata rather than silently discarded.

**Alternative considered:** Send all candidate names directly to Apple Music in one opaque request. Rejected because it loses per-candidate diagnostics and makes ordering and duplicate guarantees difficult to enforce.

### Create once, then add in bounded ordered batches

After at least one track resolves, create one playlist and add tracks in the established order, using provider/API-compatible request batches. If an add operation partially fails, return the playlist reference and detailed counts/error state rather than retrying blindly or creating another playlist.

**Alternative considered:** Retry the complete operation automatically. Rejected for the MVP because retries can duplicate provider-side writes and require idempotency state that does not yet exist.

### MusicKit JS authorization

Use MusicKit JS in the browser for user authorization and playlist writes. The local server supplies only the required developer configuration or signed setup material; MusicKit manages the user authorization session in the browser. Expose authorization status separately from recommendation data. If authorization is absent or expired, the action returns an actionable error and does not mutate the batch.

**Alternative considered:** Require a server-side user token in environment variables. Rejected because it is less appropriate for the browser-based local workflow and creates unnecessary long-lived secret handling.

### Web-only MVP surface

Expose the action through the existing local web UI/API first. The response should contain playlist status, URL/reference, added count, skipped/unresolved candidates, and provider errors. No MCP tool is required for the MVP unless the existing web route needs to reuse one internally.

**Alternative considered:** Add an MCP playlist tool as the primary surface. Deferred because the stated user workflow is the recommendation web experience and an authenticated write tool would expand the MCP security surface.

### One opening action with internal fallback

Represent Apple Music destinations with the canonical web URL as the stable value and an internal app-opening strategy. On macOS desktop, the single click action attempts the app scheme and uses a bounded fallback to the web URL; on Windows/Linux it opens the web URL directly. The UI renders one action, not two competing links.

**Alternative considered:** Replace the web URL with an app-scheme URL or show both links. Rejected because schemes are not uniformly supported and two visible choices conflict with the requested UX.

### Partial playlist writes and manual retry

Persist or return the created playlist reference and outstanding candidate identifiers when insertion is partial. A retry request targets that existing playlist and only the outstanding tracks; it is initiated by the user and never creates a second playlist.

**Alternative considered:** Automatically retry the full batch. Rejected because provider-side writes may not be idempotent and automatic retries can duplicate tracks.

## Risks / Trade-offs

- [MusicKit JS authorization/configuration complexity] → Isolate browser authorization behind an interface, fail closed, and document developer-token configuration and local-origin requirements.
- [Provider search ambiguity] → Prefer verified candidate metadata and exact/provider identifiers where available; report unresolved tracks instead of guessing.
- [Partial provider writes] → Return the created playlist reference with added/omitted counts and provider error details; retain outstanding additions for explicit manual retry only.
- [Credential leakage] → Never serialize tokens into batch records or responses; redact authorization headers and token values from logs and errors.
- [Provider rate limits or API limits] → Bound lookup and add request sizes, honor context cancellation, and surface a recoverable provider error to the user.
- [Browser action replay] → Require a persisted batch ID and explicit POST action; implementation should avoid automatic retries that could create duplicate playlists.
- [Deep-link behavior varies by browser and macOS configuration] → Keep canonical web URLs internally, invoke app links only from explicit user gestures, and treat deep-link failure as a normal fallback rather than a playlist or recommendation failure.
- [App-scheme URL formats may differ between song and playlist destinations] → Centralize destination conversion and cover both formats with platform-aware tests before exposing the UI action.

## Migration Plan

1. Add the provider/configuration boundary with a disabled-by-default or unconfigured state.
2. Add the playlist action endpoint and response types without changing recommendation generation behavior.
3. Add the Individual Song UI action and status rendering; hide or disable it when authorization is unavailable while preserving direct links.
4. Configure MusicKit JS developer credentials locally and validate against the user’s own Apple Music account using an explicitly enabled integration check.
5. Roll back by disabling/removing the route and UI action; existing recommendation and link behavior remains functional because playlist creation is additive.

## Open Questions

- The exact MusicKit JS developer-token delivery mechanism can be selected during implementation as long as user authorization remains browser-based and secrets are not persisted in recommendation data.
- The exact timestamp format can be finalized during implementation as long as it is concise, deterministic, and editable before creation.
- The exact app-scheme conversion and bounded fallback timing can be finalized during implementation as long as one visible action handles both paths.