## Purpose

Defines the relational discovery and adjacency engine: MCP tools that ground live MusicBrainz search in the local affinity matrix, the LLM recommendation strategy and bias controls that prevent anchoring and hallucination, and the local web feedback surface that closes the loop on verified candidates.

## Requirements

### Requirement: get_taste_adjacencies tool
The `get_taste_adjacencies` tool SHALL provide the user's profile metrics to ground the LLM's external or internal knowledge searches. Inputs: `seed_artists` (array of strings, optional) specific artists to build outward from, defaulting to the user's top high-affinity artists when empty, and `target_vibe` (string, optional) a text descriptor to guide the search (e.g., "virtuosic instrumentation", "erratic rhythm section"). Execution MUST pull the dynamic artist affinity matrix and flattened micro-genre topography from the database and package them into the context window. (The full input contract is defined in the mcp-api capability.)

#### Scenario: Context built from affinity matrix
- **WHEN** `get_taste_adjacencies` is called
- **THEN** the artist affinity matrix and micro-genre topography are packaged into the context window

### Requirement: get_verified_discovery_candidates tool
The `get_verified_discovery_candidates` tool SHALL retrieve a programmatically filtered set of candidate tracks/albums from an external source or cache, guaranteeing that returned album candidates are not exact albums the user has already rated or blocked through recommendation feedback. Known artists and track-level listening history SHALL remain eligible unless the exact normalized artist+album pair is excluded. Inputs: `target_vibe` (string, optional) a raw vibe or canonical MusicBrainz genre tag, with comma-separated canonical tag lists accepted; `fallback_tags` (array of strings, optional) canonical MusicBrainz genre tags derived from an abstract vibe, taking precedence over `target_vibe`; `limit` (int, optional) max candidates, defaulting to 5. Callers MUST provide either `target_vibe` or `fallback_tags`. (The full input contract is defined in the mcp-api capability.)

Execution MUST: (1) query the database layer to build a memory-resident exclusion set keyed by normalized artist+album pairs, including albums with `albums.user_rating IS NOT NULL`, albums with durable recommendation-feedback verdicts (`disliked`, `ok`, `good`, `great`, `already_know`), and albums with `not_for_me_today` feedback from the current local calendar day; (2) search live MusicBrainz recording metadata for the normalized raw tag or an OR query over normalized fallback tags, using a bounded request context and MusicBrainz-compliant client identification and rate limiting; (3) strip out candidates whose normalized artist+album pair, including known alternate album titles, hits the exclusion set, while leaving standalone artist tokens, standalone album-title tokens, track titles, and starter-track strings non-excluding; (4) return a clean JSON envelope containing the critical recommendation instructions, the effective candidate limit, and a `candidates` slice of 100% verified real-world tracks (exact Title, Artist, Album, Runtime, Release Year) to eliminate LLM autocomplete hallucinations.

#### Scenario: Album-level exclusion set built from ratings and feedback
- **WHEN** the tool runs
- **THEN** candidates matching exact normalized artist+album exclusions from whole-album ratings, durable recommendation feedback, or same-day `not_for_me_today` feedback are stripped out

#### Scenario: Known artist and track history remain eligible
- **WHEN** a candidate artist exists in local listening history or its matched starter track appears in the library, but the exact normalized artist+album pair is not excluded
- **THEN** the candidate remains eligible

#### Scenario: MusicBrainz query uses normalized tags
- **WHEN** an abstract vibe phrase is supplied
- **THEN** it is translated to canonical fallback tags before the MusicBrainz query

#### Scenario: Verified envelope returned
- **WHEN** candidates are found
- **THEN** a JSON envelope with instructions, effective candidate limit, and verified real-world candidate records (exact title, artist, album, runtime, release year) is returned

### Requirement: log_album_rating tool
The `log_album_rating` tool SHALL commit a new album rating instantly. Execution MUST normalize the input text, find the target album (or insert a new row if it does not exist), and update the `user_rating` column directly in the `albums` table. (The full input contract is defined in the mcp-api capability.)

#### Scenario: Album rating committed
- **WHEN** `log_album_rating` is called
- **THEN** the album row's `user_rating` is updated, or a new album row is inserted with the rating

### Requirement: log_recommendation_feedback tool
The `log_recommendation_feedback` tool SHALL capture recommendation-batch reactions without forcing them into the formal album rating scale. Execution MUST normalize artist and album keys and store the reaction in `recommendation_feedback`. Durable verdicts (`disliked`, `ok`, `good`, `great`, `already_know`) MUST exclude only the exact normalized artist+album pair from future discovery. `not_for_me_today` MUST exclude only that exact album on the same local calendar day. The optional starter track MUST remain contextual metadata and MUST NOT become a discovery exclusion token. This tool MUST NOT update `albums.user_rating`. (The full input contract is defined in the mcp-api capability.)

#### Scenario: Durable feedback stored and excluded from future discovery
- **WHEN** `log_recommendation_feedback` is called
- **THEN** the reaction is stored and durable verdicts exclude the exact normalized artist+album pair from future discovery

#### Scenario: Not-today feedback stored as same-day cooldown
- **WHEN** `log_recommendation_feedback` is called with verdict `not_for_me_today`
- **THEN** the exact normalized artist+album pair is excluded only on the local calendar day the feedback row was created

#### Scenario: No album rating write
- **WHEN** `log_recommendation_feedback` is called
- **THEN** `albums.user_rating` is not modified

### Requirement: Sonic topology overlap evaluation
When processing discovery candidate payloads, the LLM MUST evaluate candidate tracks based on explicit structural elements — such as syncopation, rhythmic density, percussive attack, and arrangement complexity — rather than generic commercial genre classifications.

#### Scenario: Evaluation grounded in structural elements
- **WHEN** the LLM evaluates and filters discovery candidates
- **THEN** it evaluates explicit structural elements rather than generic commercial genre classifications

### Requirement: Musician pedigree matching
The LLM MUST cross-reference side projects, production credits, guest appearances, or shared session musicians to track lineage (e.g., tracking session players, mutual producers, or historical lineup splits).

#### Scenario: Lineage used to support recommendations
- **WHEN** a candidate shares session musicians, producers, or project lineage with the user's history
- **THEN** that lineage is used to support the recommendation

### Requirement: Pristine real-world realism
The LLM MUST NOT invent acoustic descriptions or real-world credits. It MUST NOT invent production credits (e.g., attributing tracks to artists like Adam Nolly Getgood or dynamic engineers without factual verification), and MUST NOT call pop, rock, or funk tracks "down-tuned", "sludgy", or "death metal" to force a match with the user's history when those traits do not exist in reality.

#### Scenario: Fabricated credits prohibited
- **WHEN** a candidate's production credits or acoustic descriptions are not factually verified
- **THEN** the LLM does not invent them

#### Scenario: No false genre force-fitting
- **WHEN** a track is actually pop, rock, or funk
- **THEN** the LLM does not describe it with metal traits to force a profile match

### Requirement: Absolute user veto
If the user explicitly states an artist or style is "not the vibe" for the current request, that veto MUST steer the remainder of the session. Recommendation-feedback verdicts MUST remain album-scoped and MUST NOT by themselves blacklist the artist or their catalog.

#### Scenario: Explicit artist veto steers the session
- **WHEN** the user explicitly states an artist or style is "not the vibe" for the current request
- **THEN** that explicit veto steers the remainder of the session without converting album feedback into a persistent artist-catalog exclusion

### Requirement: Passive filtering versus active steering
The user's historical affinity matrix MUST function strictly as a passive filter to gauge technical complexity limits and enforce album-level local database exclusions. When the user requests a highly distinct target vibe (e.g., "Rage Against the Machine vibes"), the LLM MUST prioritize the core DNA of that requested target (e.g., funk-metal, rap-metal, staccato groove) over historical metal statistics, and MUST NOT force an unwanted heavy metal crossover onto distinct genres.

#### Scenario: Target vibe overrides historical stats
- **WHEN** the user requests a highly distinct target vibe
- **THEN** the target's core DNA is prioritized over historical profile statistics and no forced crossover onto the user's dominant genres occurs

### Requirement: Discovery payload artist and album diversity
The MCP discovery payload MUST represent a diverse array of distinct musical projects and MUST serve as the ranking input pool: at most one candidate per normalized artist/album pair and at most 2 candidates from the same normalized artist per discovery payload. Candidate ordering MAY vary between otherwise equivalent requests, but prompt-fit signals MUST take precedence over variety signals.

#### Scenario: Duplicate artist album pair rejected
- **WHEN** two candidates share the same normalized artist and album pair
- **THEN** only one of them is returned in the payload

#### Scenario: Artist candidate cap enforced in the payload
- **WHEN** more than two candidates from the same normalized artist would be included in a discovery payload
- **THEN** at most two are included

#### Scenario: Equivalent candidates vary across refreshes
- **WHEN** multiple candidates have materially equivalent prompt-fit and exclusion status
- **THEN** successive refreshes MAY return different candidates or ordering without returning an ineligible candidate

### Requirement: Displayed batch artist variety
The displayed album recommendation batch SHALL contain at most one album per normalized artist by default, unless the current prompt explicitly requests a specific artist, catalog or discography exploration, multiple albums or releases by the same artist, or a deep-dive mode. The displayed song recommendation batch MUST suppress duplicate normalized artist+album+track identities and SHOULD distribute results across artists and albums when enough eligible candidates exist. These rules MUST apply to model-selected drafts, fallback drafts, and the final post-verification batch before persistence, while still suppressing duplicate artist+album pairs. Known artists with unexcluded albums MUST remain eligible.

#### Scenario: Default batch suppresses repeated artists
- **WHEN** verified discovery produces multiple eligible albums by the same normalized artist for a normal album recommendation prompt
- **THEN** the displayed batch includes at most one album by that artist

#### Scenario: Explicit deep dive allows repeated artists
- **WHEN** the prompt explicitly asks for a catalog deep dive, discography, multiple albums from one artist, or more releases by a named artist
- **THEN** the displayed batch may include more than one album by the same normalized artist while still suppressing duplicate artist+album pairs

#### Scenario: Duplicate album pair suppressed in the displayed batch
- **WHEN** a displayed batch would contain two candidates from the same normalized artist and album pair
- **THEN** only one of them is returned

#### Scenario: Known artist with unexcluded album remains eligible
- **WHEN** a candidate is by an artist already present in local listening history but the exact normalized artist+album pair is not excluded by album rating or recommendation feedback
- **THEN** the candidate MAY appear once in the displayed batch despite artist familiarity

#### Scenario: Song batch suppresses duplicate tracks
- **WHEN** a song batch contains duplicate normalized artist, album, and track identities
- **THEN** only one identity is displayed and another eligible track is selected when available

### Requirement: Requested recommendation limit is filled when eligible candidates exist
The recommendation workflow MUST continue selecting from the complete verified candidate pool after model selection, filtering, and diversity constraints until it reaches the normalized requested limit or exhausts eligible candidates. It MUST NOT report a short batch merely because the model selected too few items or because an earlier candidate was removed by a later diversity rule.

#### Scenario: Model under-selects candidates
- **WHEN** the verified pool contains at least the requested number of eligible candidates but the model returns fewer selections
- **THEN** the workflow backfills the batch from the highest-fit remaining candidates and returns the requested number

#### Scenario: Diversity filtering removes selected candidates
- **WHEN** final artist or album diversity removes selected candidates and enough eligible alternatives remain
- **THEN** the workflow backfills from those alternatives before persistence

#### Scenario: Verified pool is genuinely too small
- **WHEN** fewer eligible candidates remain than the requested limit after exclusions, verification, and duplicate rules
- **THEN** the workflow returns all eligible candidates and exposes the actual count without inventing or duplicating candidates

### Requirement: Feedback-aware recommendation fit
Durable positive and negative recommendation feedback MUST influence ordering among otherwise eligible candidates, with recent and repeated signals weighted more heavily than stale or isolated signals. Feedback MUST NOT override exact-album exclusions, explicit avoid constraints, or clear prompt-fit requirements.

#### Scenario: Previously liked traits receive a modest preference
- **WHEN** an eligible candidate shares artist or genre traits with recently liked recommendation feedback
- **THEN** it receives a ranking preference that does not displace a substantially better prompt match solely because of feedback similarity

#### Scenario: Negative feedback reduces related repetition
- **WHEN** an eligible candidate is strongly similar to recently disliked recommendation feedback but is not the exact excluded album
- **THEN** it receives a ranking penalty while remaining eligible if the prompt explicitly requests that artist or style

#### Scenario: Feedback is absent or stale
- **WHEN** no relevant feedback exists or feedback is outside the configured recency window
- **THEN** ranking falls back to prompt fit, verified metadata, and bounded variety without failing the request

### Requirement: Feedback memory for exclusions
Durable album feedback (`disliked`, `ok`, `good`, `great`, `already_know`) MUST exclude the exact normalized artist+album pair even when the album is not in the Apple Music library and has no RYM/user rating. `not_for_me_today` MUST act only as a same-local-day exact-album cooldown. Recommendation feedback MUST NOT exclude standalone artist tokens, standalone album-title tokens, track titles, or starter-track strings.

#### Scenario: Durable feedback-only album excluded from discovery
- **WHEN** an album has durable recommendation feedback but has no library row and no RYM/user rating
- **THEN** the exact normalized artist+album pair is excluded from future discovery candidates

#### Scenario: Older not-today feedback does not exclude
- **WHEN** an album has only a `not_for_me_today` feedback row created before the current local calendar day
- **THEN** the feedback row does not exclude the album from future discovery candidates

### Requirement: Prompt ingredient preservation
Active prompt descriptors such as "funky", "groove", "weird", or "experimental" MUST survive tag planning and candidate ranking even when the user's historical profile heavily favors metal-adjacent tags.

#### Scenario: Prompt descriptors preserved in ranking
- **WHEN** the user's prompt includes active descriptors such as "funky", "groove", "weird", or "experimental"
- **THEN** those descriptors survive tag planning and candidate ranking despite a metal-heavy historical profile

### Requirement: Interpretive vibe planning
Natural-language prompts MUST be interpreted as musical intent before tag selection. The planner MUST infer energy, rhythm feel, texture, density, mood, novelty, and hard constraints, then separate true requirements from flexible vibe cues.

#### Scenario: Prompt interpreted as musical intent
- **WHEN** a natural-language prompt is processed
- **THEN** it is interpreted into musical intent dimensions (energy, rhythm feel, texture, density, mood, novelty, constraints) before tags are selected

### Requirement: Trait logic and bass proxying
Prompts that use "or" MUST be treated as any-of requests with a preference for candidates that satisfy multiple traits. Requests for "bass lines", "low end", or rhythm-section feel MUST be translated through MusicBrainz-compatible proxy tags such as `funk`, `funk rock`, `funk metal`, `groove metal`, `post-punk`, `dance-punk`, `dub`, or `jazz-funk`, because MusicBrainz rarely encodes bass performance directly.

#### Scenario: Bass request proxied to funk-adjacent tags
- **WHEN** the user requests "bass lines", "low end", or rhythm-section feel
- **THEN** MusicBrainz-compatible bass proxy tags are used

#### Scenario: Or-requests prefer multi-trait candidates
- **WHEN** a prompt uses "or"
- **THEN** it is treated as an any-of request with preference for candidates satisfying multiple traits

### Requirement: Comparison-aware prompt interpretation
When the current prompt uses named reference artists, named styles, or comparison language such as "like", "similar to", "same vibe as", "but heavier", "less", or "more", the system SHALL interpret the comparison into musical dimensions before choosing discovery tags or ranking candidates. The interpreted comparison MUST preserve reference anchors, intended comparison traits, required traits, flexible traits, false-friend traits (broad matches that only superficially fit the requested tone), and acceptable bridge traits.

#### Scenario: Reference artist qualities are decomposed
- **WHEN** a prompt asks for music like a named artist or style
- **THEN** the reference is interpreted into musical traits rather than used as a single broad tag substitute

#### Scenario: Comparative modifiers change the target
- **WHEN** a prompt asks for a reference with modifiers such as "but heavier", "less death metal", "more electronic", or "same energy but jazzier"
- **THEN** the modified traits steer tag planning and ranking instead of the unmodified reference alone

#### Scenario: False friends are preserved as negative guidance
- **WHEN** a broad genre match would satisfy a reference only superficially while missing the requested tone
- **THEN** that broad match is treated as weaker or negative evidence during ranking

### Requirement: Comparison-driven candidate ranking
The system SHALL rank verified candidates by the comparison's interpreted musical intent before broad genre-tag availability or historical taste affinity. Candidates that satisfy the comparison's narrower target traits MUST rank ahead of candidates that only satisfy a broad genre, scene, or heaviness overlap. False-friend traits MUST reduce rank unless balanced by intended or bridge traits. Historical profile data MUST calibrate quality and novelty but MUST NOT cause broad profile-aligned candidates to outrank candidates closer to the comparison target.

#### Scenario: Narrow target traits outrank broad genre overlap
- **WHEN** one candidate satisfies the comparison's narrower target traits and another only matches a broad genre, scene, or heaviness overlap
- **THEN** the candidate satisfying the narrower traits ranks ahead

#### Scenario: Historical profile does not broaden the comparison
- **WHEN** local affinity data strongly favors a broader or heavier area than the current comparison prompt requests
- **THEN** that profile data calibrates quality and novelty but does not cause broad profile-aligned candidates to outrank candidates closer to the comparison target

### Requirement: Comparison notes grounded in supported evidence
The system SHALL explain album recommendations using only traits supported by the verified candidate evidence and the interpreted comparison plan. It MUST NOT claim unsupported reference-artist similarity, genre traits, instrumentation, credits, or tone qualities. When a candidate matches through an acceptable bridge trait rather than the primary reference trait, the note MUST describe the bridge match honestly without overstating direct equivalence.

#### Scenario: Notes do not invent unsupported similarity
- **WHEN** a recommended candidate lacks evidence for a requested comparison trait
- **THEN** the note does not claim that trait or imply direct similarity on that unsupported dimension

#### Scenario: Notes describe bridge matches honestly
- **WHEN** a candidate matches the prompt through an acceptable bridge trait rather than the primary reference trait
- **THEN** the note describes the bridge match without overstating direct equivalence to the reference artist or style

### Requirement: Aggressive fallback mapping
For abstract, non-canonical, or cross-genre requests, the LLM MUST map the user's text to a broad array of precise canonical MusicBrainz tags via the `fallback_tags` parameter, which takes precedence over `target_vibe` and ensures the MusicBrainz client queries a rich, diverse slice of data. For example, `"Rage Against the Machine vibes / funky but heavy"` MUST be mapped to `fallback_tags: ["funk metal", "rap metal", "alternative metal", "funk rock"]`.

#### Scenario: Abstract request mapped to canonical tags
- **WHEN** the user supplies an abstract, non-canonical, or cross-genre request
- **THEN** it is mapped to a broad array of precise canonical MusicBrainz fallback tags

### Requirement: Knowledge transparency invariant
If a real-world track is returned via the MusicBrainz payload but the LLM's pre-training data lacks deep, explicit knowledge of its actual sonic qualities (e.g., specific instrumentation, vocal style, arrangement), the LLM MUST NOT fabricate a sonic description to make it fit the user's prompt. For highly obscure tracks where exact arrangement data is missing, the breakdown MUST pivot to verifiable historical context of the artist, label, scene, or subgenre.

#### Scenario: Obscure track uses verifiable historical context
- **WHEN** a returned track's exact arrangement data is missing
- **THEN** the LLM describes verifiable historical context (artist, label, scene, subgenre) rather than inventing sonic details

### Requirement: Concise output notes
The note for each recommended album MUST be no more than two short sentences. Excessive comma splices, semicolons, or run-on dependent clauses MUST NOT be used to bypass this restriction; thoughts MUST be broken into clean, easily scannable sentences.

#### Scenario: Notes capped at two sentences
- **WHEN** the LLM writes a note for a recommended album
- **THEN** the note is at most two short, cleanly separated sentences

### Requirement: Album-first output formatting
The system MUST present a small handful of album recommendations by default, using the exact `artist`, `album`, `release_year`, and `track_name` strings from the tool payload, with `track_name` framed as the matched track that caused the album to enter the candidate set. It MUST NOT force headings such as "Direct Adjacencies" or "Cross-Genre Wildcards" unless the user explicitly asks for categories. Each note MUST explain how the album and matched track fit the user's prompt request, without forcing fake comparisons to unrelated bands in the user's history.

#### Scenario: Recommendation uses verified payload strings
- **WHEN** a recommendation is presented
- **THEN** it uses the exact `artist`, `album`, `release_year`, and `track_name` strings from the tool payload with `track_name` framed as the matched track

#### Scenario: No forced category headings
- **WHEN** the user has not asked for categories
- **THEN** headings such as "Direct Adjacencies" or "Cross-Genre Wildcards" are not inserted

### Requirement: Web feedback surface runtime contract
The `music-vault web` command MUST serve a localhost-only recommendation chat UI that: reads artist affinity, genre topography, and recent recommendation feedback from SQLite; sends the user prompt plus compact local context to a configured local Ollama model to infer the intended vibe, separate required and flexible traits, and translate that interpretation into canonical discovery tags, preserving comparison-aware intent and enforcing a default one-album-per-artist displayed-batch diversity; routes candidate retrieval through the same `get_verified_discovery_candidates` implementation used by the MCP server, including MusicBrainz lookup, album-level local rating and recommendation-feedback exclusions, and matched-track-backed album metadata; sends only those verified MCP candidates back to Ollama for ranking and short notes, where the model returns candidate indexes rather than freeform artist/album names; persists generated batches to `recommendation_batches` and `recommendation_candidates`; and writes verdict button clicks to `recommendation_feedback` using the same canonical labels as `log_recommendation_feedback`.

#### Scenario: Verified candidates ranked by Ollama
- **WHEN** a user prompt is submitted in the web UI
- **THEN** the flow routes verified MCP candidates to Ollama, which returns candidate indexes, and the generated batch is persisted to `recommendation_batches` and `recommendation_candidates`

#### Scenario: Verdict clicks written as canonical labels
- **WHEN** a user clicks a verdict button in the web UI
- **THEN** the reaction is written to `recommendation_feedback` using the canonical labels of `log_recommendation_feedback`

### Requirement: Web product boundary
The web UI is a fast local feedback loop over the MCP discovery engine. The LLM is responsible for translating intent and ranking verified candidates, but MUST NOT invent the displayed artist, album, or starter-track fields.

#### Scenario: Displayed fields are verified candidates
- **WHEN** the web UI displays a recommendation
- **THEN** the artist, album, and starter-track fields are the verified candidate values, not LLM-generated text

### Requirement: Mode-specific web recommendation batch limits
The web recommendation API MUST apply distinct maximum batch limits by mode: Individual Song requests support an effective limit up to 20, while Album requests remain capped at 10. Both modes MUST retain the default limit of 6 for omitted or non-positive requests. Candidate retrieval and downstream processing MUST derive bounded work from the effective mode-specific limit; these web limits do not change the MCP discovery tool's independent maximum of 50.

#### Scenario: Larger song request is accepted
- **WHEN** an Individual Song request specifies a limit of 15
- **THEN** the web flow uses an effective limit of 15 rather than capping the request at 10

#### Scenario: Album request remains conservative
- **WHEN** an Album request specifies a limit of 15
- **THEN** the web flow uses an effective limit of 10

### Requirement: Balanced similar-artist seed selection
When deriving external discovery artists from multiple affinity seeds, the system SHALL consider each seed within the configured lookup bound and select eligible, normalized-unique similar artists in rounds across those seeds. A seed MUST NOT supply a second selection before each other seed with an available unique eligible result has had an opportunity to supply one, subject to the total selection limit. Existing caller-provided artist exclusions MUST remain effective.

#### Scenario: First favorite has many neighbors
- **WHEN** four affinity seeds each return distinct eligible neighbors and the selection limit is four
- **THEN** each seed contributes one neighbor even if the first seed could fill the entire limit

#### Scenario: Duplicate or excluded neighbors
- **WHEN** seed results contain repeated normalized names or names in the caller-provided exclusion set
- **THEN** those entries do not consume selection slots and selection continues through available eligible results

#### Scenario: Some seeds have no results
- **WHEN** a seed lookup fails or returns no eligible neighbors while others succeed
- **THEN** successful seeds can fill the remaining selection capacity within existing lookup bounds

### Requirement: Guaranteed prompt-tag discovery opportunity
When prompt-derived tags are available, external recording discovery SHALL attempt their search independently of artist-search result volume, unless the request context has been canceled or expired. Before album genre reconciliation, the bounded merged pool SHALL reserve half its capacity, rounded up, for unique tag-search results when available. Unfilled capacity SHALL be available to other successful sources. These collection guarantees MUST NOT impose source quotas on final recommendation ranking.

#### Scenario: Prolific artist cannot suppress tag discovery
- **WHEN** the first artist returns at least the entire merged pool capacity and the tag search has enough distinct results
- **THEN** the tag search still runs and at least half of the merged pool consists of records returned by that search

#### Scenario: Tags supply artists outside the affinity neighborhood
- **WHEN** tag results include an artist absent from the local affinity profile and similar-artist seeds
- **THEN** that artist is eligible for collection and recommendation under the same metadata, exclusion, and fit rules as other candidates

#### Scenario: Sparse or failed tag search
- **WHEN** the tag search is empty, fails, or has fewer unique records than its reserved capacity and artist searches succeed
- **THEN** artist results can use the unfilled capacity without exceeding the overall bound

#### Scenario: Similarity unavailable
- **WHEN** Last.fm is unconfigured or supplies no usable discovery artists and prompt tags are available
- **THEN** discovery uses external tag results without requiring a known-artist match

### Requirement: Fair bounded recording collection
Artist recording sources SHALL receive collection opportunities in rounds before the merged pool is truncated, with normalized seed deduplication and recording identity deduplication. A prolific source MUST NOT prevent other configured artist sources from being attempted within the bounded request context. Album and song recommendation modes SHALL use the same source-balancing policy while preserving their existing metadata processing, exact album exclusions, payload diversity limits, and prompt-fit ranking. The merged seeded pool MUST remain at most 48 recordings and MUST NOT introduce unbounded pagination or retries.

#### Scenario: Multiple prolific artists
- **WHEN** four distinct artist searches each supply sufficient unique recordings and a tag search fills its 24-record reservation
- **THEN** each artist source contributes six records to the remaining 24 positions before downstream filtering

#### Scenario: Overlapping sources
- **WHEN** a recording appears in multiple artist searches or both artist and tag results
- **THEN** it occupies only one merged-pool position and collection continues through remaining unique results to fill available capacity

#### Scenario: Only artist sources are available
- **WHEN** no tags are supplied and multiple artist searches succeed
- **THEN** the entire bounded pool is available to those sources using fair collection rounds

#### Scenario: Filtering removes reserved candidates
- **WHEN** collected candidates fail metadata validation, match album exclusions, or lose on prompt fit
- **THEN** existing filtering and ranking rules apply without retaining an ineligible or weaker candidate merely to satisfy a source quota

#### Scenario: Bounded work in both modes
- **WHEN** an album or song request includes four discovery artists and tags
- **THEN** recording retrieval attempts at most one search per distinct bounded artist seed and one tag search, obeys existing rate limiting and timeouts, and retains no more than 48 merged recordings
