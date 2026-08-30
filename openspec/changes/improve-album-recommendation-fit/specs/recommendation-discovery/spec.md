## Purpose

Defines album-first discovery behavior for verified recommendations, including batch diversity, comparison-aware prompt interpretation, candidate ranking, and feedback-safe exclusion rules.

## ADDED Requirements

### Requirement: Default album batches prefer artist variety
The system SHALL present default album recommendation batches with no more than one album per normalized artist unless the current prompt explicitly requests a specific artist, catalog exploration, multiple releases by the same artist, or a deep-dive mode. This requirement applies to both model-selected batches and fallback batches returned when model selection fails.

#### Scenario: Default batch suppresses repeated artists
- **WHEN** verified discovery produces multiple eligible albums by the same normalized artist for a normal album recommendation prompt
- **THEN** the displayed recommendation batch includes at most one album by that artist

#### Scenario: Explicit deep dive allows repeated artists
- **WHEN** the prompt explicitly asks for a catalog deep dive, multiple albums from one artist, or more releases by a named artist
- **THEN** the recommendation batch may include more than one album by the same normalized artist while still suppressing duplicate artist+album pairs

#### Scenario: Known artists remain eligible
- **WHEN** a candidate is by an artist already present in local listening history but the exact normalized artist+album pair is not excluded by album rating or recommendation feedback
- **THEN** artist familiarity alone does not remove that candidate from eligibility

### Requirement: Comparison-aware prompt interpretation
When the current prompt uses named reference artists, named styles, or comparison language such as "like", "similar to", "same vibe as", "but heavier", "less", or "more", the system SHALL interpret the comparison into musical dimensions before choosing discovery tags or ranking candidates. The interpreted comparison MUST preserve reference anchors, intended traits, required traits, flexible traits, false-friend traits to de-emphasize, and acceptable bridge traits.

#### Scenario: Reference artist qualities are decomposed
- **WHEN** a prompt asks for music like a named artist or style
- **THEN** the system interprets the reference into musical traits rather than using the artist or style name as a single broad tag substitute

#### Scenario: Comparative modifiers change the target
- **WHEN** a prompt asks for a reference with modifiers such as "but heavier", "less death metal", "more electronic", or "same energy but jazzier"
- **THEN** the modified traits steer tag planning and ranking instead of the unmodified reference alone

#### Scenario: False friends are preserved as negative guidance
- **WHEN** a broad genre match would satisfy a reference only superficially while missing the requested tone
- **THEN** that broad match is treated as weaker or negative evidence during ranking

### Requirement: Comparison-driven candidate ranking
The system SHALL rank verified candidates by the comparison's interpreted musical intent before broad genre-tag availability or historical taste affinity. Candidates that satisfy the comparison's narrower target traits MUST rank ahead of candidates that only satisfy a broad genre, scene, or heaviness overlap.

#### Scenario: Rage Against the Machine comparison preserves groove and rap-metal DNA
- **WHEN** a prompt asks for Rage Against the Machine vibes
- **THEN** candidates with funk metal, rap metal, alternative metal, rhythmic groove, or staccato riff evidence rank ahead of candidates that are merely heavy metal

#### Scenario: KNOWER comparison preserves jazz-funk and electronic fusion traits
- **WHEN** a prompt asks for something like KNOWER but heavier
- **THEN** candidates with jazz-funk, fusion, electronic, synth, groove, or rhythm-section evidence plus added weight rank ahead of generic heavy candidates that lack the fusion or groove traits

#### Scenario: Opeth comparison respects subtractive constraints
- **WHEN** a prompt asks for something like Opeth but less death metal
- **THEN** candidates with progressive, melodic, atmospheric, or dynamic contrast evidence rank ahead of candidates whose primary evidence is death-metal heaviness

#### Scenario: Symphony X and Children of Bodom comparison remains a regression case
- **WHEN** a prompt asks for virtuosic metal like Symphony X or Children of Bodom, especially symphonic metal
- **THEN** candidates with melodic, neoclassical, power, progressive, symphonic, speed, or melodic-death evidence rank ahead of candidates that only have technical death, brutal death, deathcore, grindcore, or generic death-metal evidence

#### Scenario: Historical profile does not broaden the comparison
- **WHEN** local affinity data strongly favors a broader or heavier area than the current comparison prompt requests
- **THEN** that profile data calibrates quality and novelty but does not cause broad profile-aligned candidates to outrank candidates closer to the comparison target

### Requirement: Comparison notes stay grounded in supported evidence
The system SHALL explain album recommendations using only traits supported by the verified candidate evidence and interpreted comparison plan. Notes MUST NOT claim unsupported reference-artist similarity, genre traits, instrumentation, credits, or tone qualities merely to make a candidate appear aligned.

#### Scenario: Notes do not invent unsupported similarity
- **WHEN** a recommended candidate lacks evidence for a requested comparison trait
- **THEN** the note does not claim that trait or imply direct similarity on that unsupported dimension

#### Scenario: Notes describe bridge matches honestly
- **WHEN** a candidate matches the prompt through an acceptable bridge trait rather than the primary reference trait
- **THEN** the note describes the bridge match without overstating direct equivalence to the reference artist or style
