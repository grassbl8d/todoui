# todo-ui

A fast, colourful **terminal UI for [Todoist](https://todoist.com)**, built in Go with
[Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss).

![todo-ui](docs/images/todo-ui.png)

It talks to Todoist directly over the **Sync API** and is **offline-first**: it keeps a
local cache on disk, applies your changes instantly, queues them, and pushes everything
to the server when you sync.

![todoist tasks](docs/images/todo-task-list.png)

```
 ✓ Todoist   all tasks   ⇅ due date ↑
 ▌ p1  Submit expense report
     2026-06-05 09:00  ·  #Work  ·  @finance
   p4  Read a book
     #Personal
 added: Call plumber   ●1 unsynced · online      a add · / search · s sync · …
```

---

## Features

- **Guided onboarding** — prompts for your API token on first run and validates it on
  every launch (re-prompts if it's been revoked).

![easy onboarding](docs/images/todo-ui-onboarding.png)

- **Offline-first** — works fully offline from a local cache; changes queue up and push
  on `s` (sync). A background sync runs on startup when you're online.
- **Browse** all tasks with priority-coloured markers (p1 red · p2 orange · p3 blue · p4 grey).
- **Add tasks** with natural-language quick-add (`Buy milk @errand tomorrow 9am p1`).
- **Project picker on add** with your **3 most recent projects** at the top; `A` adds
  straight to the most recent one.
- **Smart search** — plain words do an instant local text search; filter expressions
  (`today | overdue`, `#Personal & p1`, `@follow-up`) are evaluated **locally**.
- **View by Project** (`p`), **filter by priority** (`P`), **ongoing** (`o`),
  **recently added** (`R`).
- **Open & edit a task** (`Enter`) — change **priority, deadline, labels, name**,
  add **comments**, or complete it. **Due date** (`t`) and **deadline** (`D`) use
  quick-pick menus (Today / Tomorrow / Next week / recurring / Custom…).
- **Sort** by priority, due date, **deadline**, project, name, labels, or date added
  (`1`–`7`). Defaults to **date added, newest first**; set a different default in the
  `,` menu (it's remembered).
- **💡 Ideas & mind maps** — press `I` anywhere to capture and browse ideas, each
  expandable into a navigable mind map; `z` zooms a node's full text in a centered
  overlay.
- **Browser-style navigation** — `b` back, `h` home, `H`/`?` (scrollable) help.

---

## Setup (API token)

On first run, todo-ui **onboards you**: if no token is found it prompts you to paste your
Todoist API token (Todoist → **Settings → Integrations → Developer → API token**), then
validates it before continuing. The token is saved to `~/.config/todo-ui/config.json`
(and a copy to `~/.config/todoist/config.json` for CLI compatibility).

You can also provide it up front:

- **Env var:** `export TODOIST_API_TOKEN=<your token>`, or
- **File:** `~/.config/todo-ui/config.json` (or `~/.config/todoist/config.json`)
  containing `{"token": "<your token>"}`.

todo-ui looks for the token in that order: env var → `~/.config/todo-ui` →
`~/.config/todoist`. **Every launch validates the token** and re-prompts if it's been
revoked.

Verify headlessly:

```bash
todo-ui sync     # "synced: … N tasks, M projects cached"
```

---

## Building todo-ui

Requires **Go 1.24+** ([install Go](https://go.dev/dl/)).

```bash
git clone https://github.com/grassbl8d/todo-ui.git
cd todo-ui
go build -o todo-ui .      # produces ./todo-ui (todo-ui.exe on Windows)
```

> Contributing? See **[`DEVELOPING.md`](DEVELOPING.md)** for the full
> build / test / versioning / release workflow.

### Platform notes

**macOS / Linux**

```bash
go build -o todo-ui .
# optional: put it on your PATH
sudo mv todo-ui /usr/local/bin/        # Linux
mv todo-ui /opt/homebrew/bin/          # macOS (Apple Silicon Homebrew prefix)
```

**Windows** (PowerShell)

```powershell
go build -o todo-ui.exe .
# optional: move somewhere on your PATH, e.g.
Move-Item todo-ui.exe "$env:USERPROFILE\bin\todo-ui.exe"
```

### Cross-compiling

Go builds for any OS/arch from one machine — no C toolchain needed (this is pure Go):

```bash
GOOS=linux   GOARCH=amd64 go build -o dist/todo-ui-linux-amd64 .
GOOS=darwin  GOARCH=arm64 go build -o dist/todo-ui-darwin-arm64 .
GOOS=darwin  GOARCH=amd64 go build -o dist/todo-ui-darwin-amd64 .
GOOS=windows GOARCH=amd64 go build -o dist/todo-ui-windows-amd64.exe .
```

### Install directly with Go

```bash
go install github.com/grassbl8d/todo-ui@latest
# binary lands in $(go env GOPATH)/bin — make sure that's on your PATH
```

---

## Usage

Just run it:

```bash
todo-ui
```

It opens full-screen (alt-screen), loads your tasks and projects, and you drive it
entirely from the keyboard.

### Keys

| Key             | Action                                                        |
|-----------------|--------------------------------------------------------------|
| `↑` / `↓` `j`/`k` | Move selection                                             |
| `n` / `v`       | Next page / previous page (also PgDn/PgUp)                   |
| `Enter`         | **Open the task** — view & edit due, deadline, priority, labels, name |
| `a`             | Add a task — opens the **project picker** first              |
| `A`             | Add a task straight to the **most recent project**           |
| `c`             | Complete the selected task                                   |
| `x`             | Delete the selected task (asks first)                        |
| `p`             | **View by project** — pick a project; `ctrl+n` new, `ctrl+e` archive, `ctrl+d` delete |
| `P`             | **Filter by priority** — pick p1–p4 from a menu              |
| `^`             | **Pin (focus)** — show only the selected task; `:unpin` to release |
| `i` / `I`       | **💡 Catch an idea / browse ideas** (saved locally); `Enter` opens its 🗺 **mind map**, `R` renames it |
| `+`             | **Toggle light / dark theme**                                |
| `O` / `F` / `U` | **Tag / untag** the selected task as ongoing / follow-up / up next |
| `o`             | **Ongoing view** — tasks with your ongoing label (default `@ongoing`) |
| `f`             | **Follow-up view** — tasks with your follow-up label (default `@ffup`) |
| `u`             | **Up Next view** — tasks with your up-next label (default `@upnext`) |
| `t`             | **Due today** (only)                                        |
| `T`             | **Due today or earlier** (today + overdue)                  |
| `W`             | **Due this week or last week**                              |
| `m`             | **Due this month**                                          |
| `M`             | **Due this month or last month**                            |
| `d`             | **Deadline is today**                                       |
| `D`             | **Deadline is today or earlier**                            |
| `R`             | **Recently added** — the last 10 tasks you created           |
| `Y`             | **Show completed** (inside a project view only) — cycle active → active+completed → completed-only (read-only, under a separator) |
| `C`             | **Reopen** the highlighted completed task (un-complete)      |
| `/`             | **Find** — local text search, or a local filter expression  |
| `?`             | **Online search** — full Todoist filter grammar (needs network) |
| `1`–`7`         | **Sort** by priority / due / deadline / project / name / labels / date added (`0` = default; repeat to reverse) |
| `b`             | **Back** — return to the previous view (like a browser)      |
| `h` / `Esc`     | **Home** — clear all filters & views, back to all tasks      |
| `r`             | Refresh the view from the local cache                       |
| `s`             | **Sync** — push queued changes & pull updates               |
| `,`             | **Menu** — labels, auto-sync interval, date format & timezone |
| `X`             | **Clear data** — remove token, cache & queue (asks first)    |
| `` ` ``         | **Quick action** — search & run any command (type to filter, ↑/↓, Enter) |
| `~`             | **About** — logo, version & contributors                     |
| `H`             | **Help** — open the (scrollable) keyboard reference          |
| `q` / `Ctrl+C`  | Quit                                                         |

### Task view (press `Enter` on a task)

Opens a detail screen showing priority, due date, **deadline**, project, labels and
comments. From there:

| Key       | Action                                  |
|-----------|-----------------------------------------|
| `1`–`4`   | Set priority (p1–p4)                     |
| `t`       | Set / change the due date (natural language) |
| `D`       | Set the **deadline** — pick Today / Tomorrow / Next week / Next month / Custom / Clear |
| `l`       | Edit labels (comma-separated, without `@`) |
| `e`       | Edit the task name                       |
| `>`       | Add a comment (existing comments are listed above) |
| `c`       | Complete the task                        |
| `b` / `Esc` | Back to the list                      |

#### In the project picker (used by both `a` and `p`)
Your **3 most recently chosen projects** appear at the top (in gold, marked `★`),
then a separator, then all projects (blue). The cursor starts on your most recent
project. **Just start typing to filter** the list down · `↑`/`↓` to move ·
`Enter` to select · `Esc` clears the filter (or closes the picker).

### Searching — local (`/`) vs online (`?`)

**`/` is local** (instant, works offline):

- **Plain words** → case-insensitive search over task content, project, and labels,
  filtering *live* as you type. e.g. `groceries`, `call mom`.
- **Filter expressions** → evaluated against the cache. A useful subset:
  `today`, `overdue`, `no date`, `no deadline`, `deadline`, `recurring`, `@label`,
  `#project`, `p1`–`p4`, combined with `|`/`,` (or), `&` (and), `!` (not). e.g.
  `today | overdue`, `#Personal & p1`.

**`?` is online** — sends the query to Todoist and uses the **full filter grammar**
server-side, so date/relative queries that the local evaluator doesn't handle work here:
e.g. `last 7 days`, `next week`, `deadline before: +3 days`, `created before: -30 days`,
`assigned to: me`. Requires a network connection. Press `Esc` to leave results.

Press `Esc` (or `h`) to clear and return to all tasks.

### Quick label views

`o` (ongoing), `f` (follow-up) and `u` (up next) show tasks carrying a label you choose
in **Options** (`,`). Defaults: `@ongoing`, `@ffup` and `@upnext`.

### Timezone

`today`, `overdue`, and the week/month views are computed in the timezone set in
**Menu** (`,`). The default is **Asia/Manila** (UTC+8). Choosing the **Timezone** row
opens a type-to-filter picker over the IANA zone database — start typing a city or
region (e.g. `manila`, `tokyo`, `london`), use `↑/↓` to move, `enter` to select.

### Background sync

Background auto-sync defaults to every **30 seconds** (push queued changes, pull
updates). Change the interval — or turn it off with `0` — from the **Menu** (`,`).

### Pin / focus mode (for single-tasking)

Press **`P`** to pin the selected task. While pinned, todo-ui shows **only that task** and
blocks view-switching keys, so you can't wander off — handy for avoiding constant task
switching. A bright **📌 PINNED** banner is always visible with the release instructions.
To unpin, type **`:`** then **`unpin`** and Enter. Completing or deleting the pinned task
auto-unpins. The pin is **per-session** — it only affects the todo-ui instance you set it
in, so you can run several at once.

### 🗺 Mind maps

![terminal mind map](docs/images/todo-mindmap-mode.png)

Every captured idea doubles as the root of a **keyboard-driven mind map**. Press **`I`** to
browse ideas, then **`Enter`** (or `→`/`l`) on one to drop into mind-map mode. The map is
drawn as a real **left-to-right diagram** — each node is a rounded box and parents fan out to
their children through box-drawing connector lines, with the idea as the root box. The view
scrolls to follow the cursor. Maps are saved locally alongside the idea, and the idea list
shows a 🗺 badge with the node count.

```
                  ╭──────────╮     ╭───────────╮
               ┌──┤ Location ├──┬──┤ Near work │
╭─────────────╮│  ╰──────────╯  │  ╰───────────╯
│ Buy a house ├┤                └──┤ Good schools …
╰─────────────╯│  ╭────────╮
               └──┤ Budget ├──────┤ Under 500k …
                  ╰────────╯
```

| Key                | Action                                              |
|--------------------|-----------------------------------------------------|
| `↑`/`↓` · `j`/`k`  | Previous / next **sibling**                          |
| `←`/`→` · `h`/`l`  | Go to parent / descend to first child               |
| `r`                | Jump to the **root** node (left-most)               |
| `R`                | **Rename the root** node (the idea itself; ≥1 char) |
| `Tab`              | Add a **child** of the selected node                |
| `Enter`            | Add a **sibling** after the selected node           |
| `e` / `i`          | Edit the selected node's text                       |
| `z`                | **Zoom** — overlay the selected node's full text, centered over the (dimmed) map; `z`/`Esc` close |
| `Space`            | Collapse / expand a branch (`(+n)` marks hidden kids) |
| `d` / `Del`        | Delete the node and its subtree (asks y/n; a linked Todoist task is left in place) |
| `t`                | **Mark / unmark as a task** — shows a `[ ]` checkbox |
| `T`                | **Convert** marked tasks → tasks in a project (type a name to create it) |
| `U`                | **Unbind** the idea's project and unlink its tasks (asks y/n) |
| `x`                | **Complete / reopen** a task node (`[x]`); does nothing on a non-task node (use `d` to delete) |
| `s`                | **Sync** with Todoist (push tasks, pull completions) |
| `c` / `C`          | Cycle **outline** colour — `C` includes all children |
| `v` / `V`          | Cycle **background** fill — `V` includes all children |
| `` ` ``            | **Quick-action palette** (search & run any map command) |
| `H` / `?`          | Dedicated mind-map help                              |
| `Esc` / `q` / `b`  | Save and return to the idea list (`b` = back)       |

New nodes open straight into edit mode; pressing `Esc` (or leaving them blank) discards an
empty new node, so the tree never keeps placeholders. Selection is shown by **underlining**
the current node, so a node's own colour stays visible while it's selected.

**Tasks & sync.** `t` flags a node as a task; `T` turns every marked node into real Todoist
tasks under a project you pick (type a new name and it's created on the spot). An idea **binds
to a single project** — the bound name shows in the title (`🗺 Mind map → #Work`) and on the
`T` hint; pressing `T` again just tops up newly-marked tasks. Each node is **linked** to its
task, so `x` completes it (and the task on next `s` sync), and if you tick the task off in
Todoist it shows as `[x]` here after a sync. `U` unbinds the project and unlinks the generated
tasks. There are **10 node colours** that `c`/`C` (outline) and `b`/`B` (fill) cycle through.

### Theme

Press **`+`** to toggle between **dark** (default) and **light** themes; the choice is
saved.

### Add syntax (natural language)

```
Pay electricity bill @bills tomorrow 9am p1
Review PR every monday
Dentist appointment next friday 3pm
```

`@labels` and `p1`–`p4` are parsed locally; a trailing date phrase is sent to Todoist and
resolved on the next sync (until then it shows the text you typed). The **project** comes
from the picker, so multi-word project names are always exact.

---

## How sync works

todo-ui is **offline-first**:

1. On startup it loads the on-disk cache and shows it immediately (works with no network),
   then runs one background sync if you're online.
2. Every change (add / complete / delete / edit / comment) is applied to the cache
   **instantly** and appended to a pending-command queue. The footer shows `●N unsynced`.
3. Pressing **`s`** flushes the queue and pulls updates via the Todoist **Sync API**
   (incremental via a sync token). Offline, changes simply stay queued until the next sync.

Headless: `todo-ui sync` does one push+pull from a script or cron.

### State / cache files

| File | Purpose |
|------|---------|
| `~/.config/todo-ui/cache.json` | Local snapshot (tasks, projects, labels, notes, sync token) |
| `~/.config/todo-ui/queue.json` | Pending changes not yet pushed |
| `~/.config/todo-ui/recent_projects.txt` | Your last 3 chosen projects |

Delete these to reset; they're rebuilt on the next sync.

---

## Project layout

The root holds a thin `main.go` entry point; the application lives in
`internal/todoui/` (source **and** tests together, as Go requires).

| Path                          | Purpose                                                          |
|-------------------------------|-----------------------------------------------------------------|
| `main.go`                     | Thin entry point — calls `internal/todoui.Main()`               |
| `internal/todoui/main.go`     | Bubble Tea model, update loop, Lipgloss styling, `Main()`       |
| `internal/todoui/version.go`  | Build-time `version` var (stamped via `-ldflags`)               |
| `internal/todoui/sync.go`     | Todoist Sync API client, local cache, command queue, translation |
| `internal/todoui/filter.go`   | Local evaluator for the Todoist filter subset                   |
| `internal/todoui/mindmap_view.go` | 🗺 mind-map rendering (boxes, connectors, zoom overlay)      |
| `internal/todoui/*_test.go`   | Unit tests for mapping, filters, parsing, and UI state          |
| `scripts/`                    | `build-local.sh`, `run-tests.sh`, `todoist-api-test.sh`, `release.sh` |

### Running the tests

```bash
scripts/run-tests.sh        # go vet + go test ./...  (offline)
go test ./...               # or call Go directly
```

For the live Todoist API check and the full build/release workflow, see
**[`DEVELOPING.md`](DEVELOPING.md)**. Per-version changes are in
**[`RELEASE_NOTES.md`](RELEASE_NOTES.md)**.

---

## Troubleshooting

- **`no token …`** — set `TODOIST_API_TOKEN` or create
  `~/.config/todoist/config.json` with `{"token":"…"}` (see **Setup**). Verify with
  `todo-ui sync`.
- **Empty list** — run `todo-ui sync` (or press `s` in the app) to pull from the server.
- **`●N unsynced` stays** — you're offline or the token is invalid; changes are safe in
  the queue and push on the next successful sync.
- **Colours look off** — make sure your terminal advertises a 256-colour/truecolor
  `TERM` (e.g. `xterm-256color`).

---

## License

MIT — see [LICENSE](LICENSE).
