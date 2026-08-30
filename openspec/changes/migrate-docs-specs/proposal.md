## Why

The project's system behavior is currently specified by nine legacy, numbered markdown documents living in `docs/specs/`. They predate the OpenSpec workflow, so they cannot be validated against a schema, diffed as spec deltas, or linked to changes in the propose/apply cycle. Migrating them into `openspec/specs/` makes OpenSpec the single source of truth for behavioral requirements, so future changes can modify them as first-class deltas with reviewable history.

## What Changes

- Introduce nine new OpenSpec capability specs under `openspec/specs/`, one per legacy document in `docs/specs/`, converting each into the OpenSpec requirement format **without changing any behavior** (same tables, contracts, invariants, and rules).
- Remove the legacy `docs/specs/` directory once the migrated specs are in place and validated; the originals remain in git history. *(Assumption: the OpenSpec specs become the sole source of truth — see design.md.)*
- No runtime code, database schema, CLI, or API behavior changes in this change.

## Capabilities

### New Capabilities

- `data-schema`: Relational data schema (SQLite V1) — the `albums`, `tracks`, `recommendation_batches`, `recommendation_candidates`, and `recommendation_feedback` tables with their columns, constraints, and data serialization invariants.
- `ingestion`: Ingestion pipeline protocol — dynamic header sniffing, per-platform source data contracts (YouTube Music, Last.fm, Apple Music, RYM), telemetry filtering, and text normalization invariants.
- `mcp-api`: Model Context Protocol (MCP) API interface — stdin/stdout JSON-RPC transport and the registered tool contracts (`get_library_tracks`, `get_favorite_tracks`, `get_top_rated_albums`, `get_genre_distribution`, `get_album_tracks`, `get_taste_adjacencies`, `get_verified_discovery_candidates`, `log_album_rating`, `log_recommendation_feedback`).
- `metadata-enrichment`: Metadata enrichment engine (V1) — the `genres`/`track_count` schema expansion and the background enrichment worker lifecycle (target identification, rate limiting, payload extraction and commit invariants).
- `data-hygiene`: Data hygiene and compilation — the album deduplication protocol (grouping, survivor election, foreign key realignment, purge) and the `music-vault optimize` CLI integration.
- `advanced-analytics`: Advanced relational analytics and taste topography — the `v_artist_affinity` and `v_genre_topography` SQLite views with their score formulas, and the `music-vault profile` CLI hook.
- `string-normalization`: Core string normalization — the ampersand-to-"and" invariant cleaning rule, its example mappings, and the database reconciliation/migration pass over `clean_artist`/`clean_title`.
- `genre-focused-profiles`: Genre-focused profile queries — the `music-vault profile --genre/--limit` flag interface and the relational genre-to-artist filter join.
- `recommendation-discovery`: Relational discovery and adjacency engine — the discovery MCP tools, LLM recommendation strategy and bias controls, output contract, and the local `music-vault web` feedback surface.

### Modified Capabilities

- *(none — `openspec/specs/` is currently empty; all capability specs are new.)*

## Impact

- **Specs**: nine new files under `openspec/specs/<capability>/spec.md`.
- **Docs**: legacy `docs/specs/*.md` removed after migration; no code references the path (verified — only cross-references between the legacy documents themselves).
- **Code/API/Dependencies**: none affected.
- **Process**: future behavior changes to these areas will flow through the standard OpenSpec change workflow instead of direct doc edits.
