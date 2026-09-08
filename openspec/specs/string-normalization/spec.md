## Purpose

Defines the core string normalization rule that unifies connector tokens across platform naming variants — specifically the ampersand symbol versus the word "and" — and the database reconciliation pass that regenerates clean keys.

## Requirements

### Requirement: Connector-uniform invariant cleaning rule
The core string normalization function responsible for outputting `clean_artist` (and `clean_title`) MUST apply an explicit preprocessing pipeline: (1) convert the entire incoming string to lowercase; (2) explicitly replace all occurrences of the ampersand character `"&"` (and any surrounding whitespace) with the unified literal word string `" and "` before evaluating downstream punctuation strips, ensuring uniformity across symbol-based and text-based naming variants without losing the connector token; (3) proceed with standard punctuation stripping, space compression, and trim logic.

#### Scenario: Ampersand unified to the word "and"
- **WHEN** "King Gizzard & The Lizard Wizard" is normalized
- **THEN** the result is "king gizzard and the lizard wizard"

#### Scenario: Existing "and" variant unchanged
- **WHEN** "King Gizzard and The Lizard Wizard" is normalized
- **THEN** the result is "king gizzard and the lizard wizard"

### Requirement: Clean key reconciliation pass
The system MUST execute a data refresh pass across the database that recalculates and overwrites all `clean_artist` and `clean_title` columns using the updated normalization rule.

#### Scenario: Existing clean keys regenerated
- **WHEN** the reconciliation pass runs
- **THEN** every `clean_artist` and `clean_title` value is recalculated with the updated rule and overwritten

### Requirement: View join verification after migration
After the reconciliation pass runs, the `v_artist_affinity` view MUST naturally resolve the `avg_rym_rating` join for King Gizzard, successfully combining track curation data with RYM rating data.

#### Scenario: King Gizzard rows join across sources
- **WHEN** the reconciliation pass has run
- **THEN** `v_artist_affinity` resolves the King Gizzard rating join, combining track curation data with RYM rating data
