---
name: release-todo-ui
description: Cut a todo-ui release. Use when the user asks to release, cut a version, ship, or publish a new version of todo-ui. Finalizes RELEASE_NOTES.md for the version, then runs scripts/release.sh (which versions, tests, signs/notarizes, tags, publishes, and bumps to the next -dev snapshot). Claude runs the release; the user does not run release.sh themselves.
---

# Releasing todo-ui

This project releases with **one script** — `scripts/release.sh` — wrapped by a
hand-curated changelog step. **You (Claude) run the script**; the user asks for
the release, they don't run it themselves.

Versioning is a Maven-style snapshot flow: between releases `main.go`'s
`var version` carries a `-dev` suffix (e.g. `v0.2.2-dev`). A release strips it to
a clean `vX.Y.Z`, tags/publishes that, then bumps to the next `-dev`. The git tag
and GitHub release are always clean (never `-dev`). Full mechanics:
`DEVELOPING.md` and `RELEASING.md`.

## Rules (do not skip)

- **No AI attribution anywhere** — not in commits, tags, the GitHub release,
  or `RELEASE_NOTES.md`. This is the user's standing global rule.
- **Never hand-edit `var version`** in `main.go` — `release.sh` owns it.
- **Update `RELEASE_NOTES.md` before tagging.** gh's auto-notes are not enough;
  the user wants curated per-version notes.
- **Confirm the resolved version with the user** before anything is pushed
  (the script also prompts once, but state the version up front).

## Steps

1. **Commit pending work, then preflight.** `release.sh` refuses to run on a
   dirty tree, so **part of doing a release is making the commits**: review
   `git status`/`git diff` and commit any outstanding feature/fix/doc changes
   first (focused commits, **no AI attribution**). Then run the tests:
   ```bash
   scripts/run-tests.sh
   ```
   If a Todoist token is present, the release also runs the live API guard
   (`scripts/todoist-api-test.sh`); mention that.

2. **Resolve the version.** Read `var version` in `internal/todoui/version.go`
   (e.g. `v0.2.2-dev`); the clean release is that minus `-dev` (→ `v0.2.2`).
   Confirm it with the user. To force a version: `scripts/release.sh v0.3.0`.

3. **Finalize `RELEASE_NOTES.md`.** Convert the top `## Unreleased — vX.Y.Z-dev`
   heading into a dated, finalized `## vX.Y.Z — YYYY-MM-DD` section (use today's
   date), tightening the wording into real release notes. Then add a fresh empty
   `## Unreleased — v<next>-dev` section above it for the next cycle. Commit this:
   ```bash
   git add RELEASE_NOTES.md && git commit -m "Release notes for vX.Y.Z"
   ```
   (No AI attribution in the message.)

4. **Run the release.** This is the whole cycle in one command:
   ```bash
   scripts/release.sh
   ```
   It resolves the version, runs `go test ./...` + the Todoist guard, builds and
   signs/notarizes the macOS binaries, builds Linux/Windows archives, writes
   `SHA256SUMS`, prints the artifacts, and **prompts once** before pushing. After
   the prompt it tags the clean `vX.Y.Z`, pushes, creates the GitHub release,
   rebuilds the local `./todo-ui`, and bumps `main.go` to the next `-dev`.

   Useful flags (only if the user asks):
   - `scripts/release.sh vX.Y.Z` — force a specific version.
   - `scripts/release.sh --skip-mac` — Linux/Windows only (no cert/notarize).
   - `scripts/release.sh --no-publish` — build into `dist/`, push nothing.
   - `scripts/release.sh --tag-only` — just tag + push, no build.
   - `SKIP_INTEGRATION=1` — skip the live Todoist guard even with a token.

5. **Report.** Give the user the release URL the script prints and confirm
   `main.go` is now on the next `-dev` snapshot.

## Requirements / gotchas

- `SIGN_IDENTITY` must be exported in the shell for signing (the keychain may
  hold more than one Developer ID cert — the script refuses to guess). See
  `RELEASING.md`/`SIGNING.md`. If the cert isn't set up, offer `--skip-mac`.
- `gh` must be authenticated (`gh auth status`).
- A stray local-only tag can exist; the script won't reuse a version. If it
  reports a higher local tag, surface that to the user.
