# Specification 09: Relational Discovery & Adjacency Engine

## 1. Objective
Establish an MCP tool framework that handles complex, multidimensional artist and genre recommendations and allows instant local rating updates. This engine anchors your local Ollama instance using your existing database metrics and explicit authority domains to hunt for music, verify history, and log ratings directly.

## 2. MCP Tool Schema Contract

### A. Tool: `get_taste_adjacencies`
Provides your profile metrics to ground the LLM's external or internal knowledge searches.
- **Input Parameters:**
  - `seed_artists` (array of strings, optional): Specific artists to build outward from. If empty, defaults to the user's top high-affinity artists.
  - `target_vibe` (string, optional): A text descriptor to guide the search (e.g., "virtuosic instrumentation," "erratic rhythm section").
- **Execution Logic:** Pulls your dynamic artist affinity matrix and flattened micro-genre topography from the database and packages it into the context window.

### B. Tool: `check_album_history`
Allows the LLM to verify if an explored or candidate album already has a historical footprint.
- **Input Parameters:** `artist` (string, required), `album` (string, required)
- **Execution Logic:** Queries the `albums` table using Specification 07 string normalization rules (`clean_artist` and `clean_title`). Returns whether a `user_rating` exists, or if the artist has existing track affinity.

### C. Tool: `log_album_rating`
Enables you or the LLM to commit a new album rating instantly to the database.
- **Input Parameters:**
  - `artist` (string, required)
  - `album` (string, required)
  - `rating` (float, required)
- **Execution Logic:** Normalizes the input text, finds the target album (or inserts a new row if it doesn't exist), and updates the `user_rating` column directly in the `albums` table.

## 3. LLM Recommendation Strategy (The Relational Invariants)
When processing requests via `get_taste_adjacencies`, the LLM must filter, rank, and source recommendations using three structural pillars:

1. **The Sourcing Hierarchy (Priority Domains):**
   The LLM must actively prioritize searching or cross-referencing information from these specific authority domains depending on the requested vibe:
   - *Metal Depth:* Encyclopaedia Metallum (`metal-archives.com`), Shreddit Release Tracker.
   - *Progressive / Rock / Fusion:* ProgArchives (`progarchives.com`), Fecking Bahamas (`feckingbahamas.com`).
   - *Roots / Virtuosic Acoustic:* Bluegrass Today (`bluegrasstoday.com`), No Depression (`nodepression.com`).
   - *Hip Hop / Rap / Production:* HipHopDX (`hiphopdx.com`), Passion of the Weiss (`passionweiss.com` - independent/avant-garde rap), Dead End Hip Hop (`deadendhiphop.com`).
   - *Experimental / Electronic / Avant-Garde:* The Quietus (`thequietus.com`), Resident Advisor (`residentadvisor.net`).
   - *Historical Canon:* Internal knowledge of *1001 Albums You Must Hear Before You Die* and its community-curated variations.

2. **The Musician Pedigree Match:** Cross-reference side projects, guest appearances, or shared session musicians utilizing personnel logs from the priority domains.

3. **Sonic Topology Overlap:** Match based on complex time signatures, syncopation, or dense arrangement styles rather than standard commercial genre boxes.

## 4. Output Contract
The tool must return a curated discovery list grouped by connection type:
- **Direct Adjacencies:** Artists heavily tied to your core pillars.
- **Cross-Genre Wildcards:** Artists in entirely separate genres that match the exact technical complexity or execution of your top tier.