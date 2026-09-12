## MODIFIED Requirements

### Requirement: Requested recommendation limit is filled when eligible candidates exist
The recommendation workflow SHALL treat the normalized requested limit as a maximum target and backfill only from verified candidates that meet the same prompt-fit qualification as model selections. Catalog identity, broad genre overlap, and artist similarity alone MUST NOT establish specific performance traits. The workflow MUST return a smaller or empty batch when insufficient candidates qualify, with a plain-language reason and machine-readable shortfall status.

#### Scenario: Model under-selects candidates
- **WHEN** the model returns fewer selections and remaining verified candidates satisfy the same fit qualification
- **THEN** the workflow backfills only qualifying candidates up to the requested limit

#### Scenario: Diversity filtering removes selected candidates
- **WHEN** final diversity removes selected candidates
- **THEN** only qualifying alternatives are used before persistence

#### Scenario: Verified pool is genuinely too small
- **WHEN** fewer qualifying eligible candidates remain than the limit
- **THEN** the workflow returns those candidates and explains the shortfall without inventing or duplicating candidates

#### Scenario: No sufficiently supported match
- **WHEN** catalog candidates exist but none satisfy fit qualification
- **THEN** the response reports no sufficiently supported matches without presenting weak candidates as successful recommendations

### Requirement: Sonic topology overlap evaluation
When evaluating candidates, the system SHALL distinguish requested structural qualities, sourced entity-level evidence, artist-level adjacency, genre proxies, and uncertain model inference. It MUST prioritize evidence relevant to the requested musical feel and MUST NOT label specific instrumentation, technique, rhythm, or arrangement as verified solely from genre tags.

#### Scenario: Evaluation grounded in structural elements
- **WHEN** sourced structural evidence is available for a candidate
- **THEN** the evaluation uses it with its source and entity scope

#### Scenario: Evidence consists only of genre tags
- **WHEN** a candidate has a progressive-metal tag but no evidence of lead playing
- **THEN** virtuosity remains unverified and the explanation does not claim demonstrated lead-playing qualities

## ADDED Requirements

### Requirement: Free-form and structured reference inputs
The recommendation workflow SHALL continue accepting existing free-form requests without structured reference fields. It SHALL accept optional structured artist, album, or song examples and extract references from request text within the existing planning call. Explicit examples MUST take precedence over conflicting model interpretations of the same entity. The effective references MUST preserve supplied identity, entity scope, polarity, origin, and resolution status, and MUST be retained alongside the original request and extracted plan for inspection. Model-generated identity claims MUST NOT be treated as verified metadata.

#### Scenario: Plain-text reference request
- **WHEN** a request names Symphony X in its message and omits structured examples
- **THEN** planning extracts that reference and makes it available to discovery without requiring a reference form or an additional model call

#### Scenario: Explicit example overrides inference
- **WHEN** an explicit negative example is interpreted as positive by the model
- **THEN** reconciliation retains the user's negative intent and prevents that example from becoming a positive discovery seed

#### Scenario: Existing request has no references
- **WHEN** an existing caller submits only the original request fields without named references
- **THEN** the request remains valid and follows the existing no-reference fallback policy

#### Scenario: Unverified project identity
- **WHEN** the model supplies an identity that available metadata does not verify
- **THEN** the reference remains unresolved or ambiguous with its supplied text intact rather than acquiring a verified identity from model output

### Requirement: Current references lead artist discovery
Explicit current-request artist references and applicable user-provided fit examples SHALL lead bounded direct-neighbor discovery. Historical affinity seeds SHALL be used only when no usable explicit positive reference exists. Each neighbor MUST retain its source, reference identity, and available similarity score. Distinct projects MUST NOT be silently merged, and multiple references MUST NOT be replaced with a shared fixed trait bundle.

#### Scenario: Symphony X reference
- **WHEN** the request names Symphony X and the similarity provider is available
- **THEN** discovery queries neighbors of that reference rather than unrelated top historical artists

#### Scenario: Multiple reference projects
- **WHEN** Symphony X and Children of Bodom are supplied
- **THEN** their identities and neighbor evidence remain distinct and any overlap retains both provenance paths

#### Scenario: Similarity unavailable
- **WHEN** the provider is unconfigured, times out, or returns no usable neighbors for explicit references
- **THEN** the system uses bounded prompt-tag fallback, records the reason, and does not substitute unrelated historical seeds or fail solely because similarity is unavailable

### Requirement: Request-relevant personal context
Planning and selection SHALL receive bounded local context selected for relevance to the current request and references, including available track favorites, album ratings, listening summaries, taste feedback, and contextual fit feedback. Listening frequency MUST NOT be represented as an explicit fit judgment. Unrelated historical preference MUST NOT override the active request.

#### Scenario: Relevant library evidence exists
- **WHEN** the library contains preferences around the requested references
- **THEN** the context includes those records with identity and signal type rather than only global genre totals

#### Scenario: Sparse personal evidence
- **WHEN** no relevant local evidence exists
- **THEN** the system proceeds with explicit references and marks personal context as sparse without fabricating preferences

### Requirement: Entity-specific catalog selection
Related artists SHALL be candidate discovery leads, not sufficient grounds for recommending arbitrary catalog entries. Album and song candidates MUST be evaluated at their respective entity scope; artist or album evidence MUST NOT be silently promoted into verified track evidence. Existing verification, exclusions, diversity, and song destination behavior MUST remain effective.

#### Scenario: Related artist has heterogeneous catalog
- **WHEN** catalog entries carry different available fit evidence
- **THEN** selection evaluates those entries separately and does not automatically select the first recording or album

#### Scenario: Song has only album evidence
- **WHEN** a song lacks track-specific evidence
- **THEN** its assessment identifies that uncertainty and does not claim the entire album's qualities are verified for that song

### Requirement: Explicit degraded selection and bounded work
Selection failure or malformed model output MUST be distinguishable from normal selection in local diagnostics and the user response. A fallback MUST apply the same qualification rules. Requests MUST retain deadlines, cancellation, rate limits, bounded candidate collection, and bounded external calls without recursive similarity traversal or per-candidate model calls.

#### Scenario: Model selection fails
- **WHEN** selection times out or produces invalid output
- **THEN** the result identifies degraded selection and returns only qualified fallback candidates or a clear empty outcome

#### Scenario: Provider is slow
- **WHEN** an external lookup exhausts its time budget
- **THEN** cancellation and bounded fallback prevent unbounded retries or catalog expansion
