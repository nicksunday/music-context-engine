## Context

- `openspec/specs/` was initialized empty (spec-driven schema, flat layout, no existing capability paths to preserve).
- Nine legacy, numbered behavior documents live in `docs/specs/`. They are self-contained: no code or README references the `docs/specs/` path (verified by search), though the documents cross-reference each other (e.g., ingestion "Section 5", mcp-api "Specification 07").
- See proposal.md for the motivation (adopting OpenSpec as single source of truth).
- The 9 specs in this change (specs/**) are already modeled as `ADDED Requirements` per capability and mirror the source documents.

## Goals / Non-Goals

**Goals:**

- Establish a flat, stable capability layout that survives future changes.
- Preserve every behavioral rule from the legacy docs (tables, contracts, formulas, bias controls) verbatim in intent, expressed as testable requirements.
- Produce spec files that pass `openspec validate --strict` (scenario headings at exactly `####`, purpose ≥ 50 chars, requirement scenarios present).

**Non-Goals:**

- Designing the actual db schema, ingestion code, MCP server, enrichment worker, analytics views, or web UI — those behaviors are already specified and implemented.
- Re-architecting capability boundaries beyond the 1:1 legacy-doc mapping.
- Any runtime code, schema, CLI, or API behavior change.

## Decisions

1. **Flat capability layout.** Each legacy doc maps 1:1 to a top-level capability (e.g., `data-schema`, `ingestion`, `mcp-api`, `recommendation-discovery`). Alternative considered: grouped hierarchy (`library/schema`, `recommendation/discovery`) — rejected because the project has no existing openspec organization to derive grouping from, and a flat 1:1 mapping keeps review diffs against the legacy docs trivial.
2. **Drop numeric prefixes.** Legacy doc names (`01_schema.md`) carry ordering prefixes; OpenSpec capability paths are unnumbered kebab-case, so the prefixes are dropped. The full legacy name → capability path mapping is documented in proposal.md.
3. **Prose → requirement/scenario inlining.** Legacy prose (invariant lists, formulas, execution invariants, bias controls) is converted into `SHALL`/`MUST` requirements with `WHEN`/`THEN` scenarios, rather than preserved as literal prose blocks. This satisfies the spec-driven schema and makes each rule independently testable.
4. **Overlapping tool contracts (mcp-api vs recommendation-discovery).** The `mcp-api` capability owns the full wire-level tool input contracts; `recommendation-discovery` restates discovery-engine semantics (exclusion set, semantic fallback, rating boundary, bias controls) and points at `mcp-api` for shared input contracts. Alternative considered: full duplication in both specs — rejected as a maintenance burden; both capabilities are created in this same change, so the cross-reference resolves at archive time.
5. **Legacy docs disposition.** After apply and validation, the legacy `docs/specs/` directory is deleted so OpenSpec is the lone source of truth. Alternative considered: keep `docs/specs/` alongside — rejected because dual sources will drift. Originals remain in git history; rollback is a `git checkout` away.

## Risks / Trade-offs

- [Wording drift during translation from prose to requirement format] → Each spec was written directly against its source document; review by diffing `specs/<capability>/spec.md` against `docs/specs/<doc>.md` before applying.
- [Deleting `docs/specs/` could surprise anyone reading the repo] → The removal is the last task, gated on `openspec validate --strict` passing; content stays recoverable in git history and is explicitly flagged in proposal.md.
- [Cross-capability reference to `mcp-api` inside `recommendation-discovery`] → Both capabilities are created by this single change, so no archive ordering hazard; if `mcp-api` is ever renamed, the reference must be updated.

## Migration Plan

1. Author the 9 capability delta specs (done — `specs/<capability>/spec.md`).
2. Run `openspec validate --change migrate-docs-specs --strict`; fix any formatting violations (scenario heading depth, purpose length).
3. Run `/opsx-apply` to move the deltas into `openspec/specs/<capability>/spec.md` (apply requires `tasks`, already included).
4. Verify with `openspec validate` at the repo root; optionally run `openspec doctor`.
5. Final task: remove the legacy `docs/specs/` directory once the migrated specs are confirmed.

## Open Questions

- None. The capability mapping, flat layout, and legacy-doc removal are recorded as decisions above and can be adjusted before applying.