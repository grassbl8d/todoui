# Release notes

Per-version changes for todo-ui. Newest first. Dates are when the version was
tagged. Versioning follows a Maven-style snapshot flow (see
[`DEVELOPING.md`](DEVELOPING.md)): between releases `main.go` carries a `-dev`
suffix for the upcoming version.

## Unreleased — v0.2.5-dev

### Changed (breaking — keybindings)
- **Home is now `.`** (was `h`) on every screen — the task list, detail, ideas
  list, help, options, and the mind map. This frees **`h`** for navigation
  (vim-style "left/parent") everywhere, including the task list.
- **The settings menu (`,`) is now global** — open it from any screen, and
  closing it returns you to where you were (not always the task list).
- Home now also **resets the sort** to the default chosen in the menu, not just
  the filters.
- (From the mind-map work earlier this cycle) node **delete** is `Backspace`/`Del`
  (the `d` confirmation is gone — `u` undoes), **complete a task node** is `X`,
  and colours are `o/O` outline · `f/F` font · `g/G` background · `y/Y` style.

### Added
- **Sort bar in the task list**: a dedicated footer row shows the sort options
  (`1`–`7`) with the active one highlighted and its `↑`/`↓` direction — mirroring
  the mind map's styling bar.
- **Reset mind-map styles (`a`/`A`)**: `a` clears the selected node's outline /
  font / background colour and text style; `A` clears the node and all its
  children. On the root (the whole map) it asks `y`/`n` first; `u` undoes it.
- **Mind-map header**: the mind-map screens now show their own **❖ Mind map**
  header (with the idea name and bound project) instead of the Todoist task bar —
  they're local ideas, not Todoist items. The `todo-ui <version>` badge in the
  top-right is restyled.
- **Floating overview**: the mind-map overview (`Z`) is now a plain, full-screen
  floating map — no header, indicator, status, or footer. `Z`/`esc` still close.
- **Mind-map node formatting**: in addition to the **outline** colour (`o`/`O`),
  nodes now have a **font/text colour** (`f`/`F`), a **background fill**
  (`g`/`G`, moved off `f`), and a **text-style cycle** (`y`/`Y` — normal → bold →
  italic → underline). Lowercase applies to the node, uppercase to the node and
  all its children. A dedicated **🎨 styling row** in the mind-map footer always
  shows the outline / font / background / style controls plus the colour palette,
  so changing a node's look is one glance away (also listed in the `H` help).
- **Longer, configurable mind-map node labels**: labels now truncate at 48 chars
  by default (was 26), and a new **"Node label width"** entry in the `,` menu
  cycles 24 / 36 / 48 / 64 / 80.
- **Full-label toggle (`d`) in the mind map**: shows every node's full,
  untruncated text in the normal (navigable) map — the same display as the
  overview, but you can still move around and edit. `d` again restores
  truncation.

## v0.2.4 — 2026-06-29

### Added
- **Delete confirmation in the ideas list**: pressing `x` on an idea now opens a
  proper centered **modal** (over the dimmed list) asking `y`/`n` before removing
  it (and its mind map), instead of deleting instantly.
- **Global `p` / `h` shortcuts**: `p` jumps to the projects list (view-by-project)
  from anywhere — task detail, help, options, the ideas list, and the mind map;
  `h` goes home to the task list from the help/options screens too. (In the mind
  map `h` stays parent-navigation, so use `p` there or `b` to step back.)
- **Mind-map node reordering**: `Shift+↑` / `Shift+↓` swap the selected node with
  its previous / next sibling, and `Shift+←` promotes a node to become a sibling
  of its parent (outdent).
- **Fast node entry (standard outliner keys)**: while editing a node, **Enter**
  accepts it and opens the next **sibling**, **Tab** accepts and opens a **child**
  — chain a whole branch without leaving edit mode. `Esc` (or Enter on an empty
  entry) finishes.
- **Auto-clearing status line**: transient status messages (e.g. `synced`,
  `pasted as child`) now disappear on their own. A new **"Status auto-clear"**
  entry in the `,` menu sets the delay (3 / 5 / 10 / 30 seconds, or off; default
  5s).
- **Mind-map overview (`Z`)**: a true full-screen, all-branches-expanded,
  read-only survey of the whole map — the app header/title are hidden and only
  the footer shortcuts show. Node labels are shown in **full** (no truncation),
  and `/` search with `n`/`N` works inside it. Navigate to pan, `Z`/`esc` to
  close.
- **Mind-map search (`/`)**: find nodes by text; `n` / `N` cycle to the next /
  previous match (wrapping), with a match counter in the status line.
- **Mind-map undo (`u`)**: undoes the last change to the map — paste, cut,
  delete, reorder, promote, colour, rename, etc. — one step at a time (history is
  per-map session).
- **Instant node delete (`Backspace` / `Delete`)**: deleting a node no longer
  asks for confirmation — it removes the node immediately and the status line
  reminds you `u` undoes it. The old `d` binding is freed for future use.
- **Mind-map cut / copy / paste**: `x` cuts the selected node + subtree to a
  clipboard, `c` copies it, `v` pastes as a child and `V` pastes as a sibling
  (deep copy; pasted copies are unlinked from Todoist). To make room, the keys
  they displaced moved: complete/reopen a task node is now **`X`**, outline
  colour is **`o`/`O`**, and background fill is **`f`/`F`**.
- **Configurable mind-map selection underline**: the line under the selected
  node is now a bright colour (default **yellow**), and a new **"Mind-map
  underline"** entry in the `,` menu cycles through colours and remembers the
  choice.

### Fixed
- **Duplicate projects after the mind-map `T` / bind flow**: a project
  auto-created via `T` was left in the local cache under its temporary id even
  after the real one synced, so it showed up twice in the project list. Temp
  projects/labels are now cleared on sync (matching items/notes), and orphaned
  `tmp-` entries are swept on launch and after every sync.
- **Collapsed-children indicator on long labels**: the `(+N)` count (shown even
  for a single child) is now a separate, cyan badge drawn after the label, so a
  long node name can no longer truncate it away; it also no longer blends into
  the node text.
- **Mind map `T` with no marked tasks**: now shows a visible alert ("no tasks
  yet — press t on a node to mark it as a task first") instead of silently doing
  nothing. The mind map also surfaces transient status messages generally.

## v0.2.3 — 2026-06-26

### Added
- **Group subheaders in the task list**: under a sort, tasks are grouped with a
  right-aligned, non-selectable subheader showing the sort field's value (the
  date for date added / due / deadline, `P1`–`P4` for priority, the project,
  first letter for name, the labels). Shown only when there are ≥2 groups; the
  manual order has none. Navigation skips the subheaders.
- **Roadmap**: added [`ROADMAP.md`](ROADMAP.md) (Pomodoro, countdown timer,
  recurring-task support, image attachments), linked from the README.

### Fixed
- **Ideas list**: `i` now captures a new idea (it previously did nothing despite
  the "press i to catch one" hint), capturing lands back on the ideas list, and
  the keyboard legend (`i catch · b back · h home`) shows even when empty.
- **Mind map**: `I` jumps back to the ideas list (its parent); while editing a
  node, `I` is still typed text.

## v0.2.2 — 2026-06-26

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
  it cuts a clean `vX.Y.Z` tag/release, then bumps the version to the next
  `-dev`. A new `--dry-run` flag validates a release (tests + compile every
  target) and prints the plan while changing nothing.
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
