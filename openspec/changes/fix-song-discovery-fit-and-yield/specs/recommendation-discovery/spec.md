## MODIFIED Requirements

### Requirement: Discovery payload artist and album diversity
The MCP discovery payload MUST represent a diverse array of distinct musical projects and MUST serve as the ranking input pool: album mode retains at most one candidate per normalized artist/album pair and at most 2 candidates from the same normalized artist; song mode deduplicates recording identities, permits distinct tracks from the same album, and allows up to 4 candidates per artist in the ranking pool. Candidate ordering MAY vary between otherwise equivalent requests, but prompt-fit signals MUST take precedence over variety signals.

#### Scenario: Duplicate artist album pair rejected
- **WHEN** two album-mode candidates share the same normalized artist and album pair
- **THEN** only one of them is returned in the payload

#### Scenario: Artist candidate cap enforced in the payload
- **WHEN** more candidates from the same resolved artist would be included than the mode-specific pool cap
- **THEN** the mode-specific cap is enforced

#### Scenario: Equivalent candidates vary across refreshes
- **WHEN** multiple candidates have materially equivalent prompt-fit and exclusion status
- **THEN** successive refreshes MAY return different candidates or ordering without returning an ineligible candidate

### Requirement: Displayed batch artist variety
The displayed album recommendation batch SHALL contain at most one album per normalized artist by default, unless the current prompt explicitly requests a specific artist, catalog or discography exploration, multiple albums or releases by the same artist, or a deep-dive mode. The displayed song recommendation batch MUST suppress duplicate normalized artist+album+track identities and MUST prefer distinct artists among comparably fitting candidates, allowing at most two songs per artist by default; explicit artist deep dives may exceed that cap. These rules MUST apply to model-selected drafts, fallback drafts, and the final post-verification batch before persistence, while suppressing duplicate artist+album pairs in album mode and duplicate recording identities in song mode. Known artists with unexcluded albums MUST remain eligible.

#### Scenario: Default batch suppresses repeated artists
- **WHEN** verified discovery produces multiple eligible albums by the same normalized artist for a normal album recommendation prompt
- **THEN** the displayed batch includes at most one album by that artist

#### Scenario: Explicit deep dive allows repeated artists
- **WHEN** the prompt explicitly asks for a catalog deep dive, discography, multiple albums from one artist, or more releases by a named artist
- **THEN** the displayed batch may include more than one album by the same normalized artist while suppressing duplicate artist+album pairs in album mode and duplicate recording identities in song mode

#### Scenario: Duplicate album pair suppressed in the displayed batch
- **WHEN** a displayed album batch would contain two candidates from the same normalized artist and album pair
- **THEN** only one of them is returned

#### Scenario: Known artist with unexcluded album remains eligible
- **WHEN** a candidate is by an artist already present in local listening history but the exact normalized artist+album pair is not excluded by album rating or recommendation feedback
- **THEN** the candidate MAY appear once in the displayed batch despite artist familiarity

#### Scenario: Song batch suppresses duplicate tracks
- **WHEN** a song batch contains duplicate normalized artist, album, and track identities
- **THEN** only one identity is displayed and another eligible track is selected when available

### Requirement: Fair bounded recording collection
Artist recording sources SHALL receive collection opportunities in rounds before the merged pool is truncated, with normalized seed deduplication and recording identity deduplication. A prolific source MUST NOT prevent other configured artist sources from being attempted within the bounded request context. Album and song recommendation modes SHALL use the same source-balancing policy while preserving their existing metadata processing, exact album exclusions, payload diversity limits, and prompt-fit ranking. Album requests MUST retain the existing four-artist, one-tag-search and 48-record bounds. Song requests MUST use a limit-derived budget of min(8, max(4, ceil(limit/3))) artist searches and min(96, max(48, 5*limit)) merged recordings, with at most two distinct tag searches, and MUST NOT introduce unbounded pagination or retries. Initial collection SHOULD begin with four artist sources and one tag search; expansion MAY use the remaining budget only when eligible capacity is insufficient.

#### Scenario: Multiple prolific artists
- **WHEN** an album request has four distinct artist searches that each supply sufficient unique recordings and a tag search fills its 24-record reservation
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
- **WHEN** an album or song request includes discovery artists and tags
- **THEN** album retrieval retains its four-artist, one-tag-search and 48-record bounds, song retrieval uses min(8, max(4, ceil(limit/3))) artist searches, at most two tag searches, and min(96, max(48, 5*limit)) merged recordings, and both obey rate limiting and timeouts

#### Scenario: Song discovery expands within a finite budget
- **WHEN** initial song discovery cannot supply the requested eligible count and unused search budget remains
- **THEN** it expands across remaining artist or tag sources within the limit-derived artist and recording budgets and the two-tag-search ceiling, preserving rate limits, cancellation and tag reservation

## ADDED Requirements

### Requirement: Genre intent and performance uncertainty
The workflow MUST distinguish genre requests, preferred musical qualities, and explicit hard constraints. A genre name such as neoclassical metal MUST NOT by itself require verified instrumental technique. Missing performance evidence MUST remain uncertainty rather than a contradiction; candidates with affirmative genre or reference relevance and no hard-constraint conflict SHALL remain eligible as plausible matches with restrained explanations. Explicit hard requirements MUST NOT be relaxed to fill a batch.

#### Scenario: Broad neoclassical request
- **WHEN** the user requests neoclassical metal and verified candidates have relevant genre evidence but no performance annotation
- **THEN** the candidates remain eligible without fabricated claims of verified virtuosity

#### Scenario: Explicit hard requirement lacks support
- **WHEN** a user explicitly requires a particular performance feature and no candidate has sufficient evidence
- **THEN** the workflow reports insufficient evidence rather than claiming the requirement was met

### Requirement: Identity-grounded reference evidence
Artist similarity and metadata MUST attach to resolved provider identities when available. Name-only ambiguity MUST be recorded and MUST NOT transfer reference evidence or genre labels between namesakes. Search terms MUST NOT be presented as candidate metadata.

#### Scenario: Artists share a name
- **WHEN** a resolved reference neighbor and a returned recording have different artist identifiers despite the same display name
- **THEN** the recording does not inherit that neighbor's similarity or genre evidence

### Requirement: Contextual musicianship feedback
Request-fit feedback MUST preserve the user's distinctions between musicianship, vocals and energy without creating global genre bans or unstated hard constraints.

#### Scenario: Heavier positive discovery
- **WHEN** relevant positive feedback describes Obsidious as heavier than Symphony X but fitting through clean vocals and musicianship
- **THEN** that feedback remains positive contextual evidence and neither heaviness nor a technical-death-metal tag alone disqualifies a candidate

#### Scenario: Vocal preference is not exclusivity
- **WHEN** feedback praises clean vocals without requiring them exclusively
- **THEN** planning does not invent a clean-vocals-only constraint
