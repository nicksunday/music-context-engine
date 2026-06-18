# Specification 06: Advanced Relational Analytics & Taste Topography

## 1. Objective
Establish a suite of optimized SQLite database views designed to condense explicit personal curation markers (positive track favorites, explicit track dislikes, and local album evaluations) into high-fidelity taste profiles. These views allow the LLM recommendation engine to query high-density analytical maps, applying an updated non-linear track-and-album curve to ensure that masterpiece albums elevate an artist's profile, while preventing high-density outlier inflation and accounting for explicit negative aversion vectors.

---

## 2. Database View Schema Contracts

### A. View: `v_artist_affinity`
Computes an explicit affinity rank per artist by cross-referencing track-level curation data with album-level critical scoring, heavily scaling deliberate album reviews while enforcing safe caps on high-density outlier footprints.
- **Columns:** - `artist` (TEXT)
  - `clean_artist` (TEXT)
  - `favorite_tracks_count` (INTEGER): Direct count of rows where `tracks.is_favorite = 1`.
  - `disliked_tracks_count` (INTEGER): Direct count of rows where `tracks.is_disliked = 1`.
  - `avg_user_rating` (REAL): Average score of the artist's rated albums.
  - `curved_affinity_score` (REAL): The principal metric used for ranking and taste anchoring.
- **Mathematical Formula Engine:**
  The view iterates through every album by an artist, calculates its individual contribution, applies a dynamic constraint, and subtracts active aversion tracking:
  
  $$\text{Base Album Contribution} = \begin{cases} 
  \text{COALESCE}(\text{albums.track\_count}, 0) + \text{fav\_tracks\_on\_album}, & \text{if } \text{albums.user\_rating} \ge 4.0 \\ 
  \text{fav\_tracks\_on\_album}, & \text{if } \text{albums.user\_rating} < 4.0 \text{ or NULL}
  \end{cases}$$

  - **Dynamic Outlier Ceiling Constraint:** Instead of checking a boolean flag, the view applies a dynamic clamp directly to the step logic:
  
  $$\text{Clamped Album Contribution} = \text{MIN}(35.0, \text{Base Album Contribution})$$

  - **Aversion Penalty Suppression:** The final aggregated score for the artist subtracts a mathematical penalty for downvoted tracks to ensure active suppression:
  
  $$\text{curved\_affinity\_score} = \left( \sum \text{Clamped Album Contributions} \right) - \left( \text{disliked\_tracks\_count} \times 5.0 \right)$$
- **Sorting:** Default sorting maps strictly descending by `curved_affinity_score`.

### B. View: `v_genre_topography`
Flattens the JSON-serialized arrays stored inside our `tracks.genres` and `albums.genres` tables to map out the density of the library's micro-genre focus areas.
- **Columns:** - `subgenre` (TEXT)
  - `total_tracks` (INTEGER)
  - `favorite_tracks_count` (INTEGER)
  - `disliked_tracks_count` (INTEGER)
  - `avg_album_rating` (REAL)
- **Execution Rule:** Employs SQLite's native `json_each` array modifier to handle the extraction and aggregation of strings inside the serialized JSON arrays across the database. Rows marked with track-level dislikes are omitted from the active strength calculation profiles.

---

## 3. CLI Command Hook
- **Command Entry:** `go run ./cmd/music-vault profile`
- **Behavior:** Queries the new analytical views and prints a clean, concise markdown text visualization directly to `stdout`.
- **Output Blueprint:**
  - A markdown table of your Top 10 High-Affinity Artists based on the newly calculated `curved_affinity_score`.
  - A markdown table of your Top 5 Densest Subgenres sorted by your explicit favorite counts.