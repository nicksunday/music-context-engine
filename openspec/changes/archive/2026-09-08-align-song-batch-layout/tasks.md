## 1. Mode-specific prompt defaults

- [x] 1.1 Define distinct song-oriented and album-oriented default prompt strings in the shared frontend mode initialization.
- [x] 1.2 Apply the selected mode default without overwriting edited prompts or prompts restored from session history.

## 2. Dedicated recommendation layout

- [x] 2.1 Adjust the shared workbench/grid sizing so the Current Batch follows the composer directly and does not reserve a large blank vertical region on dedicated pages.
- [x] 2.2 Preserve the existing song list, album card, sidecar, and narrow-screen behavior while applying the tighter layout.
- [x] 2.3 Verify empty, populated, and loading/error batch states remain visible and usable in the revised flow.

## 3. Tests and validation

- [x] 3.1 Extend frontend/static contract tests to assert distinct mode defaults and the layout hooks used by the dedicated pages.
- [x] 3.2 Run the relevant Go web tests and the full repository test suite.
- [x] 3.3 Manually inspect `/recommendations/songs` and `/recommendations/albums` at desktop and narrow widths to confirm prompt proximity, mode-specific defaults, and preserved feedback actions.