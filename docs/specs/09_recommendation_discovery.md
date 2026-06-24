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

### B. Tool: `get_verified_discovery_candidates`
Retrieves a programmatically filtered set of candidate tracks/albums from an external source or cache, strictly guaranteeing that the user has zero historical familiarity with them.
- **Input Parameters:**
  - `target_vibe` (string, optional): A raw vibe or canonical MusicBrainz genre tag. A comma-separated canonical tag list is also accepted.
  - `fallback_tags` (array of strings, optional): Canonical MusicBrainz genre tags derived from an abstract vibe. This array takes precedence over `target_vibe`.
  - `limit` (int, optional): Max candidates to return. Defaults to 5.
- **Semantic Fallback Rule:** Callers must provide either `target_vibe` or `fallback_tags`. Before calling MusicBrainz, translate abstract, non-canonical phrases into likely canonical genre tags. For example, map `"erratic rhythm section"` to `["math rock", "idm", "breakcore"]`.
- **Execution Logic:** 1. Queries the database layer to build a memory-resident exclusion set of all tracked artists and albums with `is_favorite = 1` or play counts above a strict threshold (e.g., > 5 plays).
  2. Searches live MusicBrainz recording metadata for the normalized raw tag or an OR query over normalized fallback tags, using a bounded request context and MusicBrainz-compliant client identification/rate limiting.
  3. Programmatically strips out any candidates whose artist, album, or track string hits the exclusion set (applying Spec 07 normalization rules).
  4. Returns a clean JSON envelope containing the critical recommendation instructions, effective candidate limit, and a `candidates` slice of 100% verified, real-world tracks (including exact Title, Artist, Album, Runtime, and Release Year) to eliminate LLM autocomplete hallucinations.

### C. Tool: `log_album_rating`
Enables you or the LLM to commit a new album rating instantly to the database.
- **Input Parameters:**
  - `artist` (string, required)
  - `album` (string, required)
  - `rating` (float, required)
- **Execution Logic:** Normalizes the input text, finds the target album (or inserts a new row if it doesn't exist), and updates the `user_rating` column directly in the `albums` table.

## 3. LLM Recommendation Strategy & Bias Controls

When processing discovery candidate payloads, the LLM must evaluate, filter, and justify recommendations using strict logical invariants, while actively combating profile anchoring.

### A. The Structural Invariants
1. **Sonic Topology Overlap:** Evaluate tracks based on explicit structural elements—such as syncopation, rhythmic density, percussive attack, and arrangement complexity—rather than generic commercial genre classifications.
2. **The Musician Pedigree Match:** Cross-reference side projects, production credits, guest appearances, or shared session musicians to track lineage (e.g., tracking session players, mutual producers, or historical lineup splits).
3. **Pristine Real-World Realism:** The LLM must never invent acoustic descriptions or real-world credits. Do not invent production credits (e.g., attributing tracks to artists like Adam Nolly Getgood or dynamic engineers without factual verification). Do not call pop, rock, or funk tracks "down-tuned," "sludgy," or "death metal" to force a match with the user's history if those traits do not exist in reality.

### B. Dynamic Bias & Steering Controls (Anti-Anchoring Protocol)
1. **Absolute User Veto:** If the user states an artist or style is "not the vibe" or rejects a recommendation, that artist and their entire catalog are strictly blacklisted for the remainder of the session. The LLM is forbidden from recommending different tracks by them or defending the original choice.
2. **Passive Filtering vs. Active Steering:** The user's historical affinity matrix functions *strictly* as a passive filter to gauge technical complexity limits and enforce local database exclusions. When a user requests a highly distinct target vibe (e.g., "Rage Against the Machine vibes"), the LLM must prioritize the core DNA of that requested target (e.g., funk-metal, rap-metal, staccato groove) over historical metal statistics. Do not force an unwanted heavy metal crossover onto distinct genres.
3. **Strict Monotony Ban:** The discovery list must represent a diverse array of distinct musical projects. The LLM is strictly prohibited from populating more than 2 slots of a single discovery payload with the same artist.

### C. Tool Input Pre-Processing (Semantic Fallback Rule)
1. **Aggressive Fallback Mapping:** For abstract, non-canonical, or cross-genre requests, the LLM must map the user's text to a broad array of precise canonical MusicBrainz tags via the `fallback_tags` parameter. This parameter takes precedence over `target_vibe` and ensures the MusicBrainz client queries a rich, diverse slice of data.
   * *Example:* `"Rage Against the Machine vibes / funky but heavy"` must be mapped to `fallback_tags: ["funk metal", "rap metal", "alternative metal", "funk rock"]`.

### D. The Knowledge Transparency Invariant (Anti-Gaslighting)
1. **Strict Knowledge Boundaries:** If a real-world track is returned via the MusicBrainz payload but your pre-training data lacks deep, explicit knowledge of its actual sonic qualities (e.g., specific instrumentation, vocal style, arrangement), you are strictly forbidden from fabricating a sonic description to make it fit the user's prompt.
2. **Honorable Fallback Strategy:** For highly obscure tracks where exact arrangement data is missing, pivot the breakdown to target the verifiable historical context of the artist, label, scene, or subgenre.
   * *Example Format:* "Returned via canonical tags [X]. While exact tracking arrangements are outside local parameters, [Artist] emerged from the [Year] [Scene/Subgenre] movement, mirroring the structural timeline of your request."

### E. Output Structural Enforcement
1. **Strict Sentence Limit:** The structural breakdown for each candidate MUST be exactly two sentences.
2. **No Comma-Splice Cheating:** You are prohibited from using excessive comma splices, semicolons, or run-on dependent clauses to bypass this restriction. Break your thoughts down into two clean, easily scannable, punchy sentences.

## 4. Output Contract
* **Clean Formatting:** Provide Track, Artist, Album, Runtime, and Release Year using the exact string literals from the tool payload.
* **Honest Breakdown:** Provide a concise, exactly 2-sentence structural breakdown explaining how the track fits the *user's prompt request*, without forcing fake comparisons to unrelated bands in the user's history.
