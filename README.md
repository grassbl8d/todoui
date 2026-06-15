# todoui

A fast, colourful **terminal UI for [Todoist](https://todoist.com)**, built in Go with
[Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss).

It talks to Todoist directly over the **Sync API** and is **offline-first**: it keeps a
local cache on disk, applies your changes instantly, queues them, and pushes everything
to the server when you sync.

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
- **Open & edit a task** (`Enter`) — change **priority, due date, deadline, labels,
  name**, add **comments**, or complete it.
- **Sort** by priority, due date, **deadline**, project, name, or labels (`1`–`6`).
- **Browser-style navigation** — `b` back, `h` home, `H`/`?` (scrollable) help.

---

## Setup (API token)

On first run, todoui **onboards you**: if no token is found it prompts you to paste your
Todoist API token (Todoist → **Settings → Integrations → Developer → API token**), then
validates it before continuing. The token is saved to `~/.config/todoui/config.json`
(and a copy to `~/.config/todoist/config.json` for CLI compatibility).

You can also provide it up front:

- **Env var:** `export TODOIST_API_TOKEN=<your token>`, or
- **File:** `~/.config/todoui/config.json` (or `~/.config/todoist/config.json`)
  containing `{"token": "<your token>"}`.

todoui looks for the token in that order: env var → `~/.config/todoui` →
`~/.config/todoist`. **Every launch validates the token** and re-prompts if it's been
revoked.

Verify headlessly:

```bash
todoui sync     # "synced: … N tasks, M projects cached"
```

---

## Building todoui

Requires **Go 1.24+** ([install Go](https://go.dev/dl/)).

```bash
git clone https://github.com/grassbl8d/todoui.git
cd todoui
go build -o todoui .      # produces ./todoui (todoui.exe on Windows)
```

### Platform notes

**macOS / Linux**

```bash
go build -o todoui .
# optional: put it on your PATH
sudo mv todoui /usr/local/bin/        # Linux
mv todoui /opt/homebrew/bin/          # macOS (Apple Silicon Homebrew prefix)
```

**Windows** (PowerShell)

```powershell
go build -o todoui.exe .
# optional: move somewhere on your PATH, e.g.
Move-Item todoui.exe "$env:USERPROFILE\bin\todoui.exe"
```

### Cross-compiling

Go builds for any OS/arch from one machine — no C toolchain needed (this is pure Go):

```bash
GOOS=linux   GOARCH=amd64 go build -o dist/todoui-linux-amd64 .
GOOS=darwin  GOARCH=arm64 go build -o dist/todoui-darwin-arm64 .
GOOS=darwin  GOARCH=amd64 go build -o dist/todoui-darwin-amd64 .
GOOS=windows GOARCH=amd64 go build -o dist/todoui-windows-amd64.exe .
```

### Install directly with Go

```bash
go install github.com/grassbl8d/todoui@latest
# binary lands in $(go env GOPATH)/bin — make sure that's on your PATH
```

---

## Usage

Just run it:

```bash
todoui
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
| `p`             | **View by project** — pick a project to filter the list      |
| `P`             | **Filter by priority** — pick p1–p4 from a menu              |
| `o`             | **Ongoing** — tasks with your ongoing label (default `@ongoing`) |
| `f`             | **Follow-up** — tasks with your follow-up label (default `@ffup`) |
| `T`             | **Due today or earlier** (today + overdue)                  |
| `d`             | **Deadline is today**                                       |
| `D`             | **Deadline is today or earlier**                            |
| `R`             | **Recently added** — the last 10 tasks you created           |
| `/`             | **Find** — local text search, or a local filter expression  |
| `?`             | **Online search** — full Todoist filter grammar (needs network) |
| `1`–`6`         | **Sort** by priority / due / deadline / project / name / labels (`0` = default; repeat to reverse) |
| `b`             | **Back** — return to the previous view (like a browser)      |
| `h` / `Esc`     | **Home** — back to all tasks / all projects                  |
| `r`             | Refresh the view from the local cache                       |
| `s`             | **Sync** — push queued changes & pull updates               |
| `O`             | **Options** — ongoing/follow-up labels & auto-sync interval  |
| `X`             | **Clear data** — remove token, cache & queue (asks first)    |
| `H`             | **Help** — open the (scrollable) keyboard reference          |
| `q` / `Ctrl+C`  | Quit                                                         |

### Task view (press `Enter` on a task)

Opens a detail screen showing priority, due date, **deadline**, project, labels and
comments. From there:

| Key       | Action                                  |
|-----------|-----------------------------------------|
| `1`–`4`   | Set priority (p1–p4)                     |
| `t`       | Set / change the due date (natural language) |
| `D`       | Set / change the **deadline** (`YYYY-MM-DD`, empty clears) |
| `l`       | Edit labels (comma-separated, without `@`) |
| `e`       | Edit the task name                       |
| `m`       | Add a comment (existing comments are listed above) |
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

`o` (ongoing) and `f` (follow-up) show tasks carrying a label you choose in **Options**
(`O`). Defaults: `@ongoing` and `@ffup`.

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

todoui is **offline-first**:

1. On startup it loads the on-disk cache and shows it immediately (works with no network),
   then runs one background sync if you're online.
2. Every change (add / complete / delete / edit / comment) is applied to the cache
   **instantly** and appended to a pending-command queue. The footer shows `●N unsynced`.
3. Pressing **`s`** flushes the queue and pulls updates via the Todoist **Sync API**
   (incremental via a sync token). Offline, changes simply stay queued until the next sync.

Headless: `todoui sync` does one push+pull from a script or cron.

### State / cache files

| File | Purpose |
|------|---------|
| `~/.config/todoui/cache.json` | Local snapshot (tasks, projects, labels, notes, sync token) |
| `~/.config/todoui/queue.json` | Pending changes not yet pushed |
| `~/.config/todoui/recent_projects.txt` | Your last 3 chosen projects |

Delete these to reset; they're rebuilt on the next sync.

---

## Project layout

| File           | Purpose                                                                 |
|----------------|-------------------------------------------------------------------------|
| `sync.go`      | Todoist Sync API client, local cache, command queue, model translation  |
| `filter.go`    | Local evaluator for the Todoist filter subset                           |
| `state.go`     | Persisting the recent-projects list                                     |
| `todoist.go`   | Shared `Task` / `Project` types                                         |
| `main.go`      | The Bubble Tea model, update loop, and Lipgloss styling                  |
| `app_test.go`  | Tests for mapping, filters, parsing, and UI state transitions           |

### Running the tests

```bash
go test ./...
```

---

## Troubleshooting

- **`no token …`** — set `TODOIST_API_TOKEN` or create
  `~/.config/todoist/config.json` with `{"token":"…"}` (see **Setup**). Verify with
  `todoui sync`.
- **Empty list** — run `todoui sync` (or press `s` in the app) to pull from the server.
- **`●N unsynced` stays** — you're offline or the token is invalid; changes are safe in
  the queue and push on the next successful sync.
- **Colours look off** — make sure your terminal advertises a 256-colour/truecolor
  `TERM` (e.g. `xterm-256color`).

---

## License

MIT — see [LICENSE](LICENSE).
