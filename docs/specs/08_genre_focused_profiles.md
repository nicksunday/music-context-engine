# Specification 08: Genre-Focused Profile Queries

## 1. Objective
Enhance the existing CLI profiling utility to accept dynamic filtering arguments. By offloading genre-to-artist matching onto SQLite's relational layer, the LLM recommendation engine can request a highly targeted list of top-tier affinity artists for a specific micro-genre, significantly reducing token overhead and sorting errors.

## 2. Functional Requirements

### A. CLI Flag Interface
- **Command Syntax:** `go run ./cmd/music-vault profile [--genre <string>] [--limit <int>]`
- **Behavior:** 
  - If no flags are provided, fallback to the default global Top 10 layout established in Specification 06.
  - If the `--genre` flag is specified, filter the output to display only artists who have tracks or albums matching that specific genre string (case-insensitive).
  - The `--limit` flag should control the max rows returned (defaulting to 10 if not specified).

### B. Relational Resolution Logic
To fulfill a genre-filtered query, the Go database layer must cross-reference our artist tables with the flattened arrays processed by `v_genre_topography`. 
- **The Filter Join:** The query must safely match the user-supplied string against the unpacked strings in the `tracks.genres` or `albums.genres` JSON arrays for each artist.
- **The Result Set:** Output columns must mirror the core affinity layout: `Artist`, `Favorite Tracks`, `Avg RYM`, and `Score`, sorted strictly descending by `composite_score`.

---

## 3. Example Target Workflow
When evaluating a massive list of death metal records from an external source, the LLM will invoke:
`music-vault profile --genre "death metal" --limit 5`

**Expected Output Structure:**
```text
### Top 5 Affinity Artists for Genre: "death metal"

| Rank | Artist | Favorite Tracks | Avg RYM | Score |
| ---: | --- | ---: | ---: | ---: |
| 1 | Cannibal Corpse | 94 | N/A | 470.00 |
| 2 | ... | ... | ... | ... |
