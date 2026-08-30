# Specification 09: Relational Discovery & Adjacency Engine

## 1. Objective
Establish an MCP tool framework that handles complex, multidimensional artist and genre recommendations and allows instant local rating and recommendation-feedback updates. This engine anchors your local Ollama instance using your existing database metrics and explicit authority domains to hunt for music, verify history, and log reactions directly.

## 2. MCP Tool Schema Contract

### A. Tool: `get_taste_adjacencies`
Provides your profile metrics to ground the LLM's external or internal knowledge searches.
- **Input Parameters:**
  - `seed_artists` (array of strings, optional): Specific artists to build outward from. If empty, defaults to the user's top high-affinity artists.
  - `target_vibe` (string, optional): A text descriptor to guide the search (e.g., "virtuosic instrumentation," "erratic rhythm section").
- **Execution Logic:** Pulls your dynamic artist affinity matrix and flattened micro-genre topography from the database and packages it into the context window.

### B. Tool: `get_verified_discovery_candidates`
Retrieves a programmatically filtered set of candidate tracks/albums from an external source or cache, guaranteeing that returned album candidates are not exact albums the user has already rated or blocked through recommendation feedback. Known artists and track-level listening history remain eligible unless the exact album is excluded.
- **Input Parameters:**
  - `target_vibe` (string, optional): A raw vibe or canonical MusicBrainz genre tag. A comma-separated canonical tag list is also accepted.
  - `fallback_tags` (array of strings, optional): Canonical MusicBrainz genre tags derived from an abstract vibe. This array takes precedence over `target_vibe`.
  - `limit` (int, optional): Max candidates to return. Defaults to 5.
- **Semantic Fallback Rule:** Callers must provide either `target_vibe` or `fallback_tags`. Before calling MusicBrainz, translate abstract, non-canonical phrases into likely canonical genre tags. For example, map `"erratic rhythm section"` to `["math rock", "idm", "breakcore"]`.
- **Execution Logic:** 1. Queries the database layer to build a memory-resident exclusion set keyed by normalized artist+album pairs. The set includes albums with `albums.user_rating IS NOT NULL`, albums with durable recommendation-feedback verdicts (`disliked`, `ok`, `good`, `great`, `already_know`), and albums with `not_for_me_today` feedback from the current local calendar day.
  2. Searches live MusicBrainz recording metadata for the normalized raw tag or an OR query over normalized fallback tags, using a bounded request context and MusicBrainz-compliant client identification/rate limiting. It reconciles each matched recording against its release-group genres before returning an album candidate, preferring album-level genres and discarding candidates whose album genres contradict the search; recording-level tags are only a fallback when album genres are unavailable.
  3. Programmatically strips out candidates whose normalized artist+album pair, including known alternate album titles, hits the exclusion set (applying Spec 07 normalization rules). Standalone artist tokens, standalone album-title tokens, track titles, and starter-track strings do not exclude candidates.
  4. Returns a clean JSON envelope containing the critical recommendation instructions, effective candidate limit, and a `candidates` slice of 100% verified, real-world tracks (including exact Title, Artist, Album, Runtime, and Release Year) to eliminate LLM autocomplete hallucinations.

### C. Tool: `log_album_rating`
Enables you or the LLM to commit a new album rating instantly to the database.
- **Input Parameters:**
  - `artist` (string, required)
  - `album` (string, required)
  - `rating` (float, required)
- **Execution Logic:** Normalizes the input text, finds the target album (or inserts a new row if it doesn't exist), and updates the `user_rating` column directly in the `albums` table.

### D. Tool: `log_recommendation_feedback`
Captures recommendation-batch reactions without forcing them into the formal album rating scale.
- **Input Parameters:**
  - `artist` (string, required)
  - `album` (string, required)
  - `verdict` (string, required): `disliked`, `not_for_me_today`, `ok`, `good`, `great`, or `already_know`.
  - `starter_track` (string, optional)
  - `batch_id` (string, optional)
  - `candidate_id` (string, optional)
  - `mood` (string, optional)
  - `notes` (string, optional)
- **Execution Logic:** Normalizes artist and album keys and stores the reaction in `recommendation_feedback`. Durable verdicts (`disliked`, `ok`, `good`, `great`, `already_know`) exclude only the exact normalized artist+album pair from future discovery. `not_for_me_today` excludes only that exact album on the same local calendar day. The optional `starter_track` is stored as context, not as an exclusion token.
- **Rating Boundary:** This tool MUST NOT update `albums.user_rating`. Use `log_album_rating` only when the user intends a durable 0-5 album score.

## 3. LLM Recommendation Strategy & Bias Controls

When processing discovery candidate payloads, the LLM must evaluate, filter, and justify recommendations using strict logical invariants, while actively combating profile anchoring.

### A. The Structural Invariants
1. **Sonic Topology Overlap:** Evaluate tracks based on explicit structural elements—such as syncopation, rhythmic density, percussive attack, and arrangement complexity—rather than generic commercial genre classifications.
2. **The Musician Pedigree Match:** Cross-reference side projects, production credits, guest appearances, or shared session musicians to track lineage (e.g., tracking session players, mutual producers, or historical lineup splits).
3. **Pristine Real-World Realism:** The LLM must never invent acoustic descriptions or real-world credits. Do not invent production credits (e.g., attributing tracks to artists like Adam Nolly Getgood or dynamic engineers without factual verification). Do not call pop, rock, or funk tracks "down-tuned," "sludgy," or "death metal" to force a match with the user's history if those traits do not exist in reality.

### B. Dynamic Bias & Steering Controls (Anti-Anchoring Protocol)
1. **Absolute User Veto:** If the user explicitly states an artist or style is "not the vibe" for the current request, that veto must steer the remainder of the session. Recommendation-feedback verdicts are album-scoped and do not by themselves blacklist the artist or their catalog.
2. **Passive Filtering vs. Active Steering:** The user's historical affinity matrix functions *strictly* as a passive filter to gauge technical complexity limits and enforce album-level local database exclusions. When a user requests a highly distinct target vibe (e.g., "Rage Against the Machine vibes"), the LLM must prioritize the core DNA of that requested target (e.g., funk-metal, rap-metal, staccato groove) over historical metal statistics. Do not force an unwanted heavy metal crossover onto distinct genres.
3. **Discovery-Payload & Displayed-Batch Diversity:** Discovery must represent a diverse array of distinct musical projects. The MCP discovery payload returns at most one candidate per normalized artist/album pair and at most 2 candidates from the same normalized artist, serving as the ranking input pool. The displayed web batch defaults to at most one album per normalized artist unless the prompt explicitly opts into repeated artists (a named artist, catalog/discography exploration, deep dive, or multiple releases by the same artist).
4. **Feedback Memory:** Durable album feedback (`disliked`, `ok`, `good`, `great`, `already_know`) excludes the exact album even when it is not in the Apple Music library and has no RYM/user rating. `not_for_me_today` is a same-local-day exact-album cooldown only; older `not_for_me_today` rows do not exclude the album.
5. **Prompt Ingredient Preservation:** Active prompt descriptors such as "funky", "groove", "weird", or "experimental" must survive tag planning and candidate ranking even when the user's historical profile heavily favors metal-adjacent tags.
6. **Interpretive Vibe Planning:** Natural-language prompts should be interpreted as musical intent before tag selection. The planner should infer energy, rhythm feel, texture, density, mood, novelty, and hard constraints, then separate true requirements from flexible vibe cues.
7. **Trait Logic & Bass Proxying:** Prompts that use "or" should be treated as any-of requests with a preference for candidates that satisfy multiple traits. Requests for "bass lines", "low end", or rhythm-section feel should be translated through MusicBrainz-compatible proxy tags such as `funk`, `funk rock`, `funk metal`, `groove metal`, `post-punk`, `dance-punk`, `dub`, or `jazz-funk`, because MusicBrainz rarely encodes bass performance directly.
8. **Comparison-Aware Reference Anchoring:** When the prompt uses named reference artists, named styles, or comparison phrasing (e.g., "like", "similar to", "same vibe as", "but heavier", "less", "more"), decompose the reference into musical dimensions before choosing discovery tags or ranking candidates. Preserve reference anchors, intended comparison traits, required traits, flexible traits, false-friend traits (broad matches that superficially fit but miss the requested tone), and acceptable bridge traits.
9. **Comparison-Driven Ranking:** Rank verified candidates by how well they satisfy the comparison's interpreted musical intent before broad genre-tag overlap or historical affinity. Candidates satisfying the comparison's narrower target traits rank ahead of candidates that only share a broad genre, scene, or heaviness overlap. The historical affinity matrix calibrates quality and novelty but must not let broad profile-aligned candidates outrank candidates closer to the comparison target. False-friend traits act as weak or negative evidence unless balanced by intended or bridge traits.
10. **Grounded Comparison Notes:** Explain album recommendations using only traits supported by the verified candidate evidence and the interpreted comparison plan. Do not claim unsupported reference-artist similarity, genre traits, instrumentation, credits, or tone qualities. When a candidate matches through an acceptable bridge trait rather than the primary reference trait, describe the bridge match honestly without overstating direct equivalence.

### C. Tool Input Pre-Processing (Semantic Fallback Rule)
1. **Aggressive Fallback Mapping:** For abstract, non-canonical, or cross-genre requests, the LLM must map the user's text to a broad array of precise canonical MusicBrainz tags via the `fallback_tags` parameter. This parameter takes precedence over `target_vibe` and ensures the MusicBrainz client queries a rich, diverse slice of data.
   * *Example:* `"Rage Against the Machine vibes / funky but heavy"` must be mapped to `fallback_tags: ["funk metal", "rap metal", "alternative metal", "funk rock"]`.

### D. The Knowledge Transparency Invariant (Anti-Gaslighting)
1. **Strict Knowledge Boundaries:** If a real-world track is returned via the MusicBrainz payload but your pre-training data lacks deep, explicit knowledge of its actual sonic qualities (e.g., specific instrumentation, vocal style, arrangement), you are strictly forbidden from fabricating a sonic description to make it fit the user's prompt.
2. **Honorable Fallback Strategy:** For highly obscure tracks where exact arrangement data is missing, pivot the breakdown to target the verifiable historical context of the artist, label, scene, or subgenre.
   * *Example Format:* "Returned via canonical tags [X]. While exact tracking arrangements are outside local parameters, [Artist] emerged from the [Year] [Scene/Subgenre] movement, mirroring the structural timeline of your request."

### E. Output Structural Enforcement
1. **Concise Notes:** The note for each recommended album must be no more than two short sentences.
2. **No Comma-Splice Cheating:** Do not use excessive comma splices, semicolons, or run-on dependent clauses to bypass this restriction. Break your thoughts down into clean, easily scannable sentences.

## 4. Output Contract
* **Album-First Formatting:** Present a small handful of album recommendations by default, preferring at most one album per artist unless the prompt requests a catalog/discography deep dive. Use the exact `artist`, `album`, `release_year`, and `track_name` strings from the tool payload, with `track_name` framed as the matched track that caused the album to enter the candidate set.
* **Simple Structure:** Do not force headings such as Direct Adjacencies or Cross-Genre Wildcards unless the user explicitly asks for categories.
* **Honest Breakdown:** Provide a concise note, at most 2 short sentences, grounded in verified candidate evidence and the interpreted comparison plan, explaining how the album and matched track fit the *user's prompt request* without forcing fake comparisons to unrelated bands in the user's history.

## 5. Local Web Feedback Surface
The `music-vault web` command serves a localhost-only recommendation chat UI.

### A. Runtime Contract
- Reads artist affinity, genre topography, and recent recommendation feedback from SQLite.
- Sends the user prompt plus compact local context to a configured local Ollama model to infer the intended vibe, separate required and flexible traits, and translate that interpretation into canonical discovery tags.
- Routes candidate retrieval through the same `get_verified_discovery_candidates` implementation used by the MCP server, including MusicBrainz lookup, album-level local rating and recommendation-feedback exclusions, and matched-track-backed album metadata.
- Sends only those verified MCP candidates back to Ollama for ranking and short notes. The model returns candidate indexes, not freeform artist/album names.
- Preserves comparison-aware musical intent (reference anchors, intended traits, false friends, bridge traits) through ranking so candidates matching the comparison's target traits outrank broad genre overlap, and grounds notes in supported evidence.
- Enforces displayed-batch artist diversity by keeping at most one album per normalized artist by default across model-selected, fallback, and final batches, unless the prompt explicitly requests repeated artists (catalog/discography/deep dive).
- Persists generated batches to `recommendation_batches` and `recommendation_candidates`.
- Writes verdict button clicks to `recommendation_feedback` using the same canonical labels as `log_recommendation_feedback`.

### B. Product Boundary
The web UI is a fast local feedback loop over the MCP discovery engine. The LLM is responsible for translating intent and ranking verified candidates; it is not allowed to invent the displayed artist, album, or starter-track fields.
