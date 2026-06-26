---
name: release-dry-run-todo-ui
description: Dry-run a todo-ui release — validate everything a real release does (tests, live Todoist API guard, compile every target, show the plan) WITHOUT changing anything. Use when the user wants to check/preview/validate a release, see what a release would do, or confirm it's safe before cutting it. Makes no commit, tag, push, notarization, or dist/ artifacts, so there is nothing to undo.
---

# Dry-run a todo-ui release

A dry run answers "would a release succeed, and what would it do?" **without
changing anything**. It is the safe way to check before the real cut.

It is just one command:

```bash
scripts/release.sh --dry-run
```

## What it does (all read-only)

1. Preflight (tolerates a dirty tree — it changes nothing).
2. Resolves the release version from `internal/todoui/version.go` (strips `-dev`).
3. Runs `go test ./...`, plus the live Todoist API guard **if a token is
   present** (`SKIP_INTEGRATION=1` skips it).
4. Compiles **every release target** (darwin arm64/amd64, linux amd64/arm64,
   windows amd64) to `/dev/null` — proves it builds, writes nothing.
5. Prints the plan a real release would follow (bump → tag → sign/notarize →
   GitHub release → next `-dev` bump).

## What it does NOT do

No version bump, **no commit, no tag, no push, no Apple notarization, no `dist/`
artifacts, no local binary.** Because it changes nothing, **there is nothing to
undo** — that's the whole point of a dry run.

## How to run it

1. Run `scripts/release.sh --dry-run` (add `SKIP_INTEGRATION=1` if there's no
   token or the user wants to skip the network check).
2. Relay the result: the resolved version, whether tests + builds passed, and
   the printed plan.
3. If it failed, surface the exact error (test failure, build error, etc.) — do
   NOT proceed to a real release.

## Notes

- A dry run does **not** require `gh` auth or signing certs (it never publishes
  or signs), so it works on any machine.
- To actually cut the release afterward, use the **release-todo-ui** skill
  (which also commits pending work and finalizes `RELEASE_NOTES.md`).
- `dist/` is git-ignored; a dry run won't touch it regardless.
