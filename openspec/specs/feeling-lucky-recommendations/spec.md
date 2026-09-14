# feeling-lucky-recommendations Specification

## Purpose

Defines a low-friction recommendation mode that uses the breadth of a user's listening history to produce unbiased, genre-balanced surprises without reinforcing personal preference signals.

## Requirements

### Requirement: Generate an unbiased Feeling Lucky batch

The system MUST provide a Feeling Lucky generation request that accepts a requested item count `N` and returns up to `N` verified playable song or album items. Selection MUST use normalized genres represented in the user's listening history as its baseline scope and MAY expand that scope with clearly identified genre or genre-adjacent areas inferred from the same listening profile. Observed and inferred genre buckets MUST be selected without weighting them by listening frequency, catalog size, or inference confidence. The system MUST NOT use favorites, ratings, dislikes, recommendation feedback, recency, artist affinity, album quality, or prior recommendation rank as selection weights.

#### Scenario: User generates a Feeling Lucky batch
- **WHEN** the user requests a valid count `N` from the Feeling Lucky page
- **THEN** the system returns a persisted batch containing no more than `N` verified playable items selected from observed or profile-inferred genre areas derived from the user's listening history

#### Scenario: Genre frequency does not dominate selection
- **WHEN** one historically represented genre has substantially more listening data or candidates than another
- **THEN** both genres remain equally eligible at the genre-selection step rather than the larger genre receiving proportional preference

#### Scenario: Taste signals are ignored
- **WHEN** the profile contains ratings, favorites, dislikes, feedback, affinity scores, or listening recency
- **THEN** those signals do not increase or decrease a Feeling Lucky item's selection probability

### Requirement: Permit heard items and include discovery candidates

Feeling Lucky MUST allow an item to be selected when it has previously appeared in the user's listening history, library, or prior recommendation batch. The candidate pool MUST also admit verified items not present in the user's listening history when they match a historically represented genre and are available from the configured discovery source. Existing durable exclusion rules MUST NOT be applied solely because an item was previously heard.

#### Scenario: Previously listened-to item is eligible
- **WHEN** a previously heard album or song is otherwise a valid candidate for a selected genre
- **THEN** Feeling Lucky may return that item

#### Scenario: New item is eligible
- **WHEN** a verified catalog item matches a selected historical genre and is not in local listening history
- **THEN** Feeling Lucky may return that item alongside heard items

### Requirement: Make inferred genre scope transparent

When inferred genres or genre-adjacent areas expand the candidate scope, the response and page MUST distinguish observed genres from inferred genres and MUST identify that inference was used. The system MUST NOT present an inferred genre as a directly observed listening-history fact.

#### Scenario: Inferred genres expand discovery
- **WHEN** the listening profile supports a genre or genre-adjacent area that is not explicitly present in normalized history tags
- **THEN** that inferred area may be used as a Feeling Lucky candidate scope and is labeled as inferred in the response or UI

#### Scenario: Inference confidence does not become taste weighting
- **WHEN** inferred areas have different confidence scores
- **THEN** confidence may determine whether an area is admitted to the eligible scope but does not give higher-confidence areas greater selection weight after admission

#### Scenario: Invalid or unavailable item is omitted
- **WHEN** a candidate cannot be verified or has no usable playable identity/destination
- **THEN** it is omitted and does not count toward the returned batch size

### Requirement: Bound and explain batch generation outcomes

The Feeling Lucky request MUST validate `N` against a safe positive range and MUST return a clear response describing the requested count, returned count, and the genres used for sampling. When fewer than `N` eligible verified items are available, the system MUST return the available items without fabricating candidates and MUST identify that the result was limited. When no usable historical genres or candidates are available, it MUST return an explicit empty state with an actionable explanation.

#### Scenario: Candidate pool is smaller than requested
- **WHEN** fewer than `N` eligible verified items are available
- **THEN** the response returns the smaller verified set and indicates that the batch was limited by available data

#### Scenario: Profile has no usable genres
- **WHEN** listening history contains no normalized genre values and no profile-derived inferred genre areas usable for discovery
- **THEN** no batch is generated and the UI/API returns an explicit setup or enrichment message

#### Scenario: Count is invalid
- **WHEN** `N` is zero, negative, malformed, or above the supported maximum
- **THEN** the request is rejected with a validation error and no batch is persisted

### Requirement: Present Feeling Lucky as a dedicated mode

The web UI MUST provide a dedicated Feeling Lucky page with a clearly labeled action to generate or regenerate a batch, a visible count control, and a presentation that identifies each returned item and its available direct listening destination. The page MUST show loading, empty, limited-result, and error states without silently falling back to song or album recommendation behavior.

#### Scenario: Feeling Lucky page is opened
- **WHEN** the user navigates to the Feeling Lucky destination
- **THEN** the page explains that selection is based on observed and, when available, inferred listening-profile genres rather than taste weighting, and shows the count control and generation action

#### Scenario: Feeling Lucky batch is displayed
- **WHEN** a batch is available
- **THEN** the page displays each verified item, its item type/metadata, its available listening link, and the returned-versus-requested count

#### Scenario: Generation is still in progress
- **WHEN** the user submits a Feeling Lucky request
- **THEN** the page shows a loading state and prevents duplicate generation submissions until the request completes or fails