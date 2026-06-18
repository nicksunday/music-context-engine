# Specification 07: Core String Normalization Patch

## 1. Objective
Address a data integrity mismatch where database string joins fail between multi-platform ingestion tables due to varied string representations of specific characters (specifically the ampersand `&` vs. the word `and`). This patch updates the normalization function and regenerates the schema's `clean_artist` references.

## 2. Functional Adjustments

### A. The Invariant Cleaning Rule
Locate the core string normalization function in our Go utility layer (the function responsible for outputting `clean_artist`). Update the string transformation pipeline to include an explicit preprocessing rule:
1. **Lowercase:** Convert the entire incoming string to lowercase.
2. **Connector Uniformity:** Explicitly replace all occurrences of the ampersand character `"&"` (and any surrounding whitespace) with the unified literal word string `" and "` before evaluating downstream punctuation strips. This ensures uniformity across symbol-based and text-based naming variants without losing the connector token.
3. **Character Punctuation Strip:** Proceed with standard punctuation stripping, space compression, and trim logic.

*Example Mapping:*
- `"King Gizzard & The Lizard Wizard"` $\rightarrow$ `"king gizzard and the lizard wizard"`
- `"King Gizzard and The Lizard Wizard"` $\rightarrow$ `"king gizzard and the lizard wizard"`

## 3. Database Reconciliation & Migration
- **Data Refresh Pass:** Execute an internal loop or update query across the database to recalculate and overwrite all `clean_artist` and `clean_title` columns using the updated Go normalization rule.
- **View Verification:** After running the migration, the `v_artist_affinity` view must naturally resolve the `avg_rym_rating` join for King Gizzard, successfully combining track curation data with RYM rating data.