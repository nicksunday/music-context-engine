## MODIFIED Requirements

### Requirement: get_verified_discovery_candidates tool

The `get_verified_discovery_candidates` tool SHALL continue to retrieve programmatically filtered candidate tracks/albums for prompt-driven discovery. In addition, the recommendation system MUST support a Feeling Lucky retrieval path that uses normalized genres from the user's listening history as the baseline candidate scope and MAY add genre or genre-adjacent buckets inferred from that listening profile. Observed and inferred buckets MUST be selected uniformly rather than by genre frequency, candidate volume, or inference confidence, and the path MUST NOT apply taste, rating, favorite, feedback, recency, affinity, or quality weighting. For Feeling Lucky, known artists, previously listened-to tracks, and previously heard albums SHALL remain eligible unless excluded for invalid/unavailable metadata or an explicit non-history eligibility rule; prior listening alone MUST NOT exclude an item. Newly discovered verified items matching a selected observed or inferred genre area MUST also remain eligible.

#### Scenario: Feeling Lucky uses historical genre scope
- **WHEN** Feeling Lucky requests candidates
- **THEN** the candidate retrieval scope is derived from normalized genres found in the user's listening history

#### Scenario: Feeling Lucky samples genres uniformly
- **WHEN** historical genres have unequal listening counts or unequal candidate counts
- **THEN** genre selection does not favor the genre with more records or candidates

#### Scenario: Feeling Lucky admits profile-inferred genres
- **WHEN** the listening profile supports a genre or genre-adjacent area not explicitly tagged in history
- **THEN** the area may be admitted as an identified inferred Feeling Lucky genre bucket

#### Scenario: Inferred genre confidence does not bias selection
- **WHEN** admitted inferred genre buckets have unequal inference confidence
- **THEN** each admitted bucket remains equally eligible at the genre-selection step

#### Scenario: Prior listening does not exclude candidates
- **WHEN** a verified candidate's artist, track, or exact album appears in listening history
- **THEN** it remains eligible for Feeling Lucky unless another explicit exclusion applies

#### Scenario: New discovery candidates are returned
- **WHEN** a verified external candidate matches a historical genre but is absent from local history
- **THEN** it may be included in the Feeling Lucky candidate pool

#### Scenario: Album-level exclusion set built from ratings and feedback
- **WHEN** the prompt-driven tool runs outside Feeling Lucky mode
- **THEN** candidates matching exact normalized artist+album exclusions from whole-album ratings, durable recommendation feedback, or same-day `not_for_me_today` feedback are stripped out

#### Scenario: Known artist and track history remain eligible
- **WHEN** a prompt-driven candidate artist exists in local listening history or its matched starter track appears in the library, but the exact normalized artist+album pair is not excluded
- **THEN** the candidate remains eligible

#### Scenario: MusicBrainz query uses normalized tags
- **WHEN** an abstract vibe phrase is supplied to the prompt-driven path
- **THEN** it is translated to canonical fallback tags before the MusicBrainz query

#### Scenario: Verified envelope returned
- **WHEN** candidates are found by the prompt-driven path
- **THEN** a JSON envelope with instructions, effective candidate limit, and verified real-world candidate records (exact title, artist, album, runtime, release year) is returned

#### Scenario: Prompt-driven discovery remains compatible
- **WHEN** an existing caller invokes the tool with `target_vibe` or `fallback_tags` and no Feeling Lucky mode
- **THEN** the existing prompt-driven candidate contract and album exclusion behavior remain unchanged