## ADDED Requirements

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
