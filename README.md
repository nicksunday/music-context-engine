# Music Context Platform (MCP)

A local-first, personal music intelligence platform built in Go. This project ingests historical listening data and music exports from various platforms, normalizes and enriches the metadata using open API registries, and exposes your entire musical profile over the Model Context Protocol (MCP). 

Ultimately, this serves as a high-fidelity context engine powering localized music recommendation pipelines via LLMs (like Qwen MoE models via Ollama).

## Data Gathering & Ingestion Ecosystem

This platform treats your listening history as entirely private, local-first state. The `data/` directory is excluded from version control—**you bring your own data.** The platform currently supports ingesting data exports from four major hooks:

### 1. Last.fm (Scrobbles & Loved Tracks)
* **Extraction Source:** Export logs generated via the community [Last.fm to CSV Export Tool](https://lastfm.ghan.nl/export/).
* **What it provides:** Granular listening habits, chronological behavior, and active preference tracking.
* **Ingestion Mechanics:** Processes user scrobble history and loved track logs (`data/lastfm-scrobbles.csv` and `data/lastfm-loved-tracks.csv`). It cross-references these streams against your existing database:
  * Matching tracks are dynamically flipped to `is_favorite = 1`.
  * If a loved track or scrobble doesn't exist in the library yet, the system self-heals by generating missing track/album records on the fly, defaulting missing album frames to `Unknown Album`.

### 2. RateYourMusic (RYM)
* **What it provides:** Core structural data for your album library and personal ratings.
* **Ingestion Mechanics:** Parsed via `data/rym-music-export.csv`. The pipeline automatically maps and normalizes album titles and artists. It scales your RYM star ratings down to a standard 5.0 maximum (dividing by 2) and extracts chronological release years to establish historical baselines.

### 3. YouTube Music (YTM)
* **What it provides:** Track-level library state from your cloud streaming ecosystem.
* **Ingestion Mechanics:** Extracts track and streaming metadata from your official Google Takeout uploads CSV (`data/music uploads metadata.csv`). If a track is missing explicit artist metadata during ingestion, the pipeline safely falls back to a standardized `Unknown Artist` tag to maintain relational database integrity.

### 4. Apple Music
* **Extraction Sources:** Native Apple Music playlist/library CSV-style exports and Apple Data & Privacy archive files.
* **What it provides:** Local-library sync, explicit likes/dislikes, album track counts, and playback telemetry for skip/history analysis.
* **Ingestion Mechanics:** The `music-vault ingest` command content-sniffs headers and JSON keys, so Apple Music files can be passed directly alongside other exports:
  ```sh
  music-vault ingest \
    "data/Apple Music Library Tracks.json" \
    "data/Apple Music Library Activity.json" \
    "data/Apple Music - Favorites.csv" \
    "data/Apple Music Play Activity.csv" \
    "data/Apple Music - Track Play History.csv"
  ```
  Supported Apple Music contracts are:
  * **Native likes/library CSV:** Files whose first columns are `Name,Artist,Composer,Album` are treated as positive library intent. Existing tracks are marked `is_favorite = 1` and `is_disliked = 0`; missing tracks self-heal into `tracks` and `albums`, falling back to `Unknown Artist` or `Unknown Album` when needed.
  * **Favorites CSV:** Apple Data & Privacy files with `Favorite Type`, `Item Description`, and `Preference` parse song rows such as `Artist - Title`. `LIKE`/`liked` values set `is_favorite = 1`; `DISLIKE`/`disliked` values set `is_disliked = 1` and clear favorite status.
  * **Library Tracks JSON:** `Apple Music Library Tracks.json` arrays create or update track and album rows from `Title`, `Artist`/`Album Artist`, and `Album`. `Favorite Status - Track` and `Track Like Rating` map into the same favorite/disliked flags, while `Track Count On Album` updates `albums.track_count`.
  * **Library Activity JSON:** `Apple Music Library Activity.json` transaction arrays process nested `Tracks`, including later update records that reference only `Track Identifier`. This keeps affinity flags and album track counts synchronized as Apple reports library changes over time.
  * **Play telemetry CSV:** `Apple Music Play Activity.csv` rows are stored in `apple_music_play_activity` with `end_reason_type`, `play_duration_ms`, and a computed `was_skipped` flag whenever the end reason contains `SKIP`.
  * **Track Play History CSV:** `Apple Music - Track Play History.csv` stores compact playback rows in the same telemetry table after parsing `Track Name` values formatted as `Artist - Title` and converting Apple epoch-millisecond timestamps to UTC.
* **Noise Filtering:** Play telemetry skips ambient/background signatures such as `White Noise`, `Bedtime Mix`, `Rain Sounds`, `Sleeping`, `Ocean Waves`, and `Radiance` before writing analytics facts.

---

## Metadata Enrichment Pipeline

Once raw data is ingested, the library state is extended via asynchronous metadata loops (`music-vault enrich`):
* **Artist & Genre Tagging:** Scans for records with missing genre text arrays and queries the **MusicBrainz** API.
* **Community Tag Aggregation:** If a `LASTFM_API_KEY` is present, it pulls community top-tags and passes them through a strict, internal whitelist to filter out low-value noise (e.g., filtering out tags like "seen live" or "awesome" while keeping strict subgenres).
* **Idempotency:** Empty arrays are explicitly committed for missing or unresolvable artists, ensuring the engine doesn't waste network resources or rate-limits retrying dead endpoints.

### Proactive Discovery & LLM Anti-Hallucination

Rather than relying on LLMs to blind-guess music recommendations and validating them reactively, the platform implements a proactive filtering design over the Model Context Protocol:
* **Pre-Exclusion Filtering:** The server builds an in-memory database exclusion map of your entire track, album, and artist history using Unicode string normalization rules.
* **MusicBrainz Integration:** External discovery requests leverage live MusicBrainz metadata queries wrapped in a padded fetch buffer (`limit * 4`) to prevent result starvation during data stripping.
* **Monotony & Anti-Anchoring Guardrails:** The protocol layer enforces a strict limit of 2 tracks per artist to ensure discovery variety, while forcing the LLM to prioritize your active "target vibe" over past library anchors.

## Technical Stack & Layout

* **Language:** Go 1.25.5
* **Database:** Embedded SQLite (`data/music_vault.db`) via `mattn/go-sqlite3` with strict Unicode normalization for clean search indexing.
* **Protocol Interface:** `mark3labs/mcp-go` exposing local context tools via `stdin`/`stdout`. This includes semantic discovery routing (`get_verified_discovery_candidates`), structural taste mapping (`get_taste_adjacencies`), and instantaneous local metadata logging (`log_album_rating`).
* **Local Inference Engine:** Ollama running Qwen Mixture of Experts (MoE) models to execute high-fidelity context reasoning and routing.
