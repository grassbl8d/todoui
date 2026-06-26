---
name: commit-todo-ui
description: Commit pending todo-ui changes as focused commits. Use when the user asks to commit, "do the commits", save/check in work, or stage changes for todo-ui. Groups changes into coherent commits, runs gofmt + tests first, and writes clean messages with NO AI attribution.
---

# Commit todo-ui changes

Make clean, focused commits of the working-tree changes. Follow these project
rules exactly.

## Hard rules

- **Commit only — never bump the version or anything else.** This skill just
  checks in the files that already changed. Do NOT edit
  `internal/todoui/version.go`, the changelog, or any version string. Version
  bumps happen **only during a release** (`release-todo-ui` / `release.sh`),
  which moves the source to the next `-dev` snapshot after publishing.
- **No AI attribution anywhere** — not in the subject, body, a "Generated with"
  line, or a `Co-Authored-By:` trailer. Write as the sole human author. (Standing
  global rule for this user.)
- **Don't push unless asked.** Commit locally; mention the commits aren't pushed.
- **Format + verify before committing.** Run `gofmt -w` on changed Go files and
  `scripts/run-tests.sh`; don't commit a broken or unformatted tree.

## Steps

1. **Survey.** `git status` and `git diff` (and `git diff --cached`) to see
   everything that changed and group it into logical units.

2. **Format & test.**
   ```bash
   gofmt -l internal/todoui main.go     # should print nothing
   scripts/run-tests.sh
   ```
   Fix formatting (`gofmt -w …`) and any failures before continuing.

3. **Commit in focused units**, each a coherent change with a clear message:
   - One commit per feature / fix / refactor / docs change where the files allow.
   - **Staging caveat:** this harness can't do interactive `git add -p`, so you
     can only stage whole files. When several features share one file (e.g.
     `internal/todoui/main.go`), they must go in one commit — describe them all
     in the body rather than splitting mid-file. Don't pretend they're separate.
   - Keep each commit buildable where practical (code + its tests together; Go
     files with the `go.mod` change they need).

4. **Message style.** Imperative subject, ≤ ~72 chars, no trailing period; a body
   with `- ` bullets when the change spans several things. Examples from history:
   `Fix quick-add parser: on/in/next/every aren't due dates unless a date follows`,
   `Move app into internal/todoui; thin root main.go`.

5. **Verify.** After committing, confirm the tree is clean (`git status`), the tip
   builds/tests, and **no AI attribution** slipped in:
   ```bash
   git log -N --format='%an <%ae>%n%b' | grep -iE 'claude|anthropic|co-authored' || echo "none"
   ```
   Then report the commit list to the user and note nothing is pushed.

## Notes

- The user works directly on `main` for this repo; committing there is expected.
  (If asked to push or open a PR, that's a separate explicit step.)
- This skill pairs with **release-todo-ui** — cutting a release starts by
  committing pending work (release.sh refuses a dirty tree).
