## 1. Migrate legacy specs into OpenSpec capability deltas

- [ ] 1.1 Confirm all 9 capability delta specs exist under `openspec/changes/migrate-docs-specs/specs/` and cover the behavior of their source documents: `data-schema` (01_schema.md), `ingestion` (02_ingestion.md), `mcp-api` (03_mcp_api.md), `metadata-enrichment` (04_enrichment.md), `data-hygiene` (05_data_hygiene.md), `advanced-analytics` (06_advanced_analytics.md), `string-normalization` (07_string_normalization_patch.md), `genre-focused-profiles` (08_genre_focused_profiles.md), `recommendation-discovery` (09_recommendation_discovery.md)
- [ ] 1.2 Spot-check each spec against its legacy source document in `docs/specs/` to catch any wording drift or dropped invariants
- [ ] 1.3 Run `openspec validate migrate-docs-specs --type change --strict` and fix any issues until it passes

## 2. Apply the specs into openspec/specs

- [ ] 2.1 Run the apply workflow on `migrate-docs-specs` to move the 9 deltas into `openspec/specs/<capability>/spec.md`
- [ ] 2.2 Verify all 9 capability directories exist under `openspec/specs/`: `data-schema`, `ingestion`, `mcp-api`, `metadata-enrichment`, `data-hygiene`, `advanced-analytics`, `string-normalization`, `genre-focused-profiles`, `recommendation-discovery`, each containing `spec.md`
- [ ] 2.3 Run `openspec validate --specs --strict` at the repo root and fix any issues
- [ ] 2.4 Run `openspec doctor` and resolve any reported problems

## 3. Retire the legacy docs

- [ ] 3.1 Re-verify no code or documentation references the `docs/specs/` path (search for `docs/specs`, `Specification 0`, and the old numeric doc titles)
- [ ] 3.2 Delete the legacy `docs/specs/` directory, keeping the current `docs/` tree intact (originals remain recoverable in git history)
- [ ] 3.3 Confirm the working tree only contains the intended changes (`openspec/` additions and `docs/specs/` removal) before committing