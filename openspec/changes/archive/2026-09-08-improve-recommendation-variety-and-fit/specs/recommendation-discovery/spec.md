## MODIFIED Requirements

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

#### Scenario: Song batch suppresses duplicate tracks
- **WHEN** a song batch contains duplicate normalized artist, album, and track identities
- **THEN** only one identity is displayed and another eligible track is selected when available

#### Scenario: Duplicate album pair suppressed in the displayed batch
- **WHEN** a displayed batch would contain two candidates from the same normalized artist and album pair
- **THEN** only one of them is returned

#### Scenario: Known artist with unexcluded album remains eligible
- **WHEN** a candidate is by an artist already present in local listening history but the exact normalized artist+album pair is not excluded by album rating or recommendation feedback
- **THEN** the candidate MAY appear once in the displayed batch despite artist familiarity

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