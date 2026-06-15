# todoui

A fast, colourful **terminal UI for [Todoist](https://todoist.com)**, built in Go with
[Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss).

It's a friendly front-end over the [`sachaos/todoist`](https://github.com/sachaos/todoist)
command-line client — it shells out to the `todoist` binary and parses its CSV output,
so it reuses your existing authentication and local cache. No API token wiring required.

```
 ✓ Todoist   all tasks
 ▌ p1  Pay Globe Westgrove
     #Bills Payments  ·  25/06/05(Thu) 09:00  ·  @bills-payment
   p4  Read a book
     #Personal
 a add · A add→last · / search · enter/c done · d del · r refresh · q quit
```

---

## Features

- **Browse** all tasks with priority-coloured markers (p1 red · p2 orange · p3 blue · p4 grey).
- **Add tasks easily** with natural-language quick-add (`Buy milk @errand tomorrow 9am p1`).
- **Project picker on add** — press `a` and choose the destination project from a
  type-to-filter list. The **last project you used is pre-selected**, so you can just
  hit `Enter`.
- **`A` shortcut** — add straight to the *last project* you picked, skipping the picker.
  Your last project is remembered across restarts.
- **Smart search** — type plain words for an instant **local text search** over task
  name / project / labels (filters live as you type), or type a real
  [Todoist filter](https://todoist.com/help/articles/205248842)
  (`today | overdue`, `#Personal & p1`, `@follow-up`) to run it server-side.
- **View by Project** — press `p`, pick a project from the list, and the view narrows
  to just that project's tasks.
- **Complete**, **delete** (with confirmation), and **sync + refresh** — all from the keyboard.

---

## Prerequisites

You need the **`todoist` CLI** ([`sachaos/todoist`](https://github.com/sachaos/todoist))
installed, on your `PATH`, and authenticated. Verify with:

```bash
todoist list      # should print your tasks
```

If you've never used it, run `todoist sync` once and follow the auth prompt
(it asks for your Todoist API token, found in Todoist → Settings → Integrations → Developer).

### Installing the `todoist` CLI

| Platform | Command |
|----------|---------|
| **macOS** | `brew install todoist` |
| **Linux** | Download a binary from the [releases page](https://github.com/sachaos/todoist/releases), or `go install github.com/sachaos/todoist@latest` |
| **Windows** | Download the `.exe` from [releases](https://github.com/sachaos/todoist/releases), or `go install github.com/sachaos/todoist@latest` (ensure `%GOPATH%\bin` is on `PATH`) |

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
| `a`             | Add a task — opens the **project picker** first              |
| `A`             | Add a task straight to the **last project** (skip picker)    |
| `p`             | **View by project** — pick a project to filter the list      |
| `/`             | Search — plain words (local text search) or a Todoist filter |
| `Enter` / `c`   | Complete the selected task                                   |
| `d`             | Delete the selected task (asks `y`/`n`)                      |
| `r`             | Sync + refresh                                               |
| `Esc`           | Clear the active search / project view / filter (or cancel a prompt) |
| `q` / `Ctrl+C`  | Quit                                                         |

#### In the project picker (used by both `a` and `p`)
Type to fuzzy-filter the list · `↑`/`↓` to move · `Enter` to select · `Esc` to cancel.
The cursor starts on your last-used project.

### Searching

The search bar (`/`) is **smart**:

- **Plain words** → an instant, case-insensitive **local search** over task content,
  project, and labels. It filters *live* as you type. e.g. `anvaya`, `pay globe`.
- **Filter expressions** → run server-side via the `todoist` CLI. Detected when your
  query uses operators (`|`, `&`, `#`, `@`, …) or keywords (`today`, `overdue`,
  `p1`, `7 days`, …). e.g. `today | overdue`, `#Personal & p1`.

Press `Esc` to clear and return to all tasks.

### Viewing one project

Press `p`, choose a project (type to narrow the list), and the task view filters to that
project. You can stack a text search on top with `/`. Press `Esc` to clear.

### Add syntax (Todoist quick-add)

The add bar understands Todoist's natural language:

```
Pay Globe bill @bills-payment tomorrow 9am p1
Review PR every monday
Dentist appointment next friday 3pm
```

Dates, `@labels`, and `p1`–`p4` priority are parsed by Todoist. The **project** comes
from the picker (not from `#project` text), which makes multi-word project names reliable.

> **How project routing works:** `todoist quick` parses natural language but can't
> reliably route multi-word project names. So todoui quick-adds the task (for the NL
> parsing), then moves the new task into your chosen project via `todoist modify
> --project-id`. You get both: natural language *and* exact project placement.

### Search syntax (Todoist filters)

```
today | overdue
#Personal & p1
@follow-up
search: globe
no date
7 days
```

See the [Todoist filters guide](https://todoist.com/help/articles/205248842) for the full grammar.

---

## Configuration / state

todoui remembers your last-used project so `Enter`/`A` are instant. It's stored in a
single tiny file:

| Platform | Location |
|----------|----------|
| macOS / Linux | `~/.config/todoui/last_project.txt` |
| Windows | `%USERPROFILE%\.config\todoui\last_project.txt` |

Delete that file to reset. There's no other configuration — todoui inherits everything
else (auth, cache, default project) from the `todoist` CLI.

---

## Project layout

| File           | Purpose                                                                 |
|----------------|-------------------------------------------------------------------------|
| `todoist.go`   | Thin wrapper around the `todoist` CLI (list/quick-add/close/delete/modify/sync, projects, labels) |
| `state.go`     | Persisting the last-used project                                         |
| `main.go`      | The Bubble Tea model, update loop, and Lipgloss styling                  |
| `app_test.go`  | Tests for CSV parsing and UI state transitions                          |

### Running the tests

```bash
go test ./...
```

---

## Troubleshooting

- **`todoist: executable file not found`** — the `todoist` CLI isn't on your `PATH`.
  Install it (see Prerequisites) and confirm `todoist list` works in the same shell.
- **Empty list / auth errors** — run `todoist sync` once to (re)authenticate.
- **Filter error** — your search string isn't valid Todoist filter syntax; press `Esc`
  to clear and try again.
- **Colours look off** — make sure your terminal advertises a 256-colour/truecolor
  `TERM` (e.g. `xterm-256color`).

---

## License

MIT — see [LICENSE](LICENSE).
