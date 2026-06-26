# Release notes

Per-version changes for todo-ui. Newest first. Dates are when the version was
tagged. Versioning follows a Maven-style snapshot flow (see
[`DEVELOPING.md`](DEVELOPING.md)): between releases `main.go` carries a `-dev`
suffix for the upcoming version.

## Unreleased — v0.2.2-dev

### Added
- **Mind-map zoom overlay** (`z`): floats the selected node's full, untruncated
  text in a popup centered over the map, with the map dimmed behind it for
  focus. The map stays navigable underneath (the popup follows the selection);
  `z`/`esc` close it, `H` still opens help while zoomed.
- **Due-date quick picker** (`t` in the task detail): a numbered menu mirroring
  the deadline picker (Today / Tomorrow / This weekend / Next week / Every day /
  Every Monday / Custom… / Clear), with recurring picks kept as natural language.
- **Default sort is now created-date descending** (newest first) on launch, and
  a persisted **"Default sort"** entry in the `,` menu cycles and remembers it.
- **Ideas hotkey works everywhere**: `I` opens the ideas list from the task
  list, detail view, help, about, and the menu. A `💡 I ideas` affordance now
  shows in the header and the footer.

### Fixed
- **Quick-add date parser**: `on` / `in` / `next` / `every` are no longer
  treated as due-date keywords unless followed by an actual date, so
  "Remind to add this **on** some features" keeps its full title.

### Internal
- Restructured into a thin root `main.go` plus an `internal/todoui/` package
  (source and tests together), so the repo root is no longer cluttered with
  every `.go` file. Build/version ldflags retarget `internal/todoui.version`.

### Tooling & docs
- `scripts/release.sh` now manages the `-dev` snapshot version automatically:
  it cuts a clean `vX.Y.Z` tag/release, then bumps `main.go` to the next
  `-dev`.
- New `scripts/build-local.sh` (build + test locally, no signing) and
  `scripts/run-tests.sh` (unit-test runner); `integration-test.sh` renamed to
  `scripts/todoist-api-test.sh` to make its Todoist scope clear.
- New `DEVELOPING.md` (build / test / versioning / release guide); signing
  identity details removed from the committed release docs.

## v0.2.1 — 2026-06-16

- Live Todoist API integration guard (behind the `integration` build tag) to
  catch endpoint breakage before a release.
- Keyboard mind maps, a completed-task view, and sync UX improvements.
- `release.sh` rebuilds the local `./todo-ui` after publishing so symlinked
  installs pick up the new version immediately.

## v0.1.7 — 2026-06-16

- Sort by date added (`7`).
- Quick-action palette (`` ` ``) and an about screen (`~`).

## v0.1.6 — 2026-06-16

- One-command release script + `RELEASING.md` guide.
- Customizable timezone (default Asia/Manila); background auto-sync default 30s.
- "Up Next" tagging with `u` / `U`.
- `b` closes the Options/Menu; clearer theme-toggle hint.

## v0.1.5 — 2026-06-16

- Deadline quick-pick, configurable date format, project archive, home flash.
- Key updates: pin → `^`, priority → `P`, Menu → `,`, tag views `O`/`F`,
  filter `S`; project add/delete; idea catcher.

## v0.1.0–v0.1.2 — 2026-06-16

Initial public releases:

- Colourful terminal UI for Todoist, rewritten on the Todoist Sync API:
  offline-first with a local cache and command queue (no CLI dependency).
- Token onboarding + validation; token stored in `~/.config/todoui`.
- Task detail/edit view, sorting, smart search, view-by-project, priority
  filter, comments, recently-added view.
- Pin/focus mode, light/dark theme toggle, browser-style back/home navigation,
  scrollable help page.
- `--version` / `--help` flags; macOS code-signing + notarization tooling.
