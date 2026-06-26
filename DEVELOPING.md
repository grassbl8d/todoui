# Developing todo-ui

A practical guide for working on todo-ui: how to build it, how to test it
(including the live Todoist API check), how versioning works, and how to cut a
release. For end-user install/usage see [`README.md`](README.md); for the
one-time Apple signing setup see [`SIGNING.md`](SIGNING.md); for the full
release reference see [`RELEASING.md`](RELEASING.md).

## Prerequisites

- **Go 1.24+** (`go.mod` pins `go 1.24.2`). Pure Go — no C toolchain needed.
- `git`.
- For cutting releases: `gh` (authenticated), and on macOS the Apple
  Developer ID cert + notary profile (see `SIGNING.md`).

The module path is `github.com/grassbl8d/todo-ui`; the whole app is a flat
package `main` at the repo root.

## Build

The quickest path — vet, run the unit tests, and produce a native `./todo-ui`:

```bash
scripts/build-local.sh            # vet + unit tests + native build
scripts/build-local.sh --run      # ...then launch ./todo-ui
scripts/build-local.sh --skip-tests
scripts/build-local.sh --help
```

This does **no** signing, notarizing, or publishing — it's purely for trying a
build on your machine. The binary reports the in-repo version (see
[Versioning](#versioning)).

Or build by hand:

```bash
go build -o todo-ui .             # native build → ./todo-ui
./todo-ui                         # run it
go run .                          # build + run in one step
```

Cross-compiling (pure Go, from any host):

```bash
GOOS=linux   GOARCH=amd64 go build -o dist/todo-ui-linux-amd64 .
GOOS=darwin  GOARCH=arm64 go build -o dist/todo-ui-darwin-arm64 .
GOOS=windows GOARCH=amd64 go build -o dist/todo-ui-windows-amd64.exe .
```

## Test

There are two layers of tests.

### Unit tests (offline, fast)

```bash
scripts/run-tests.sh              # go vet + go test ./...  (the usual entry point)
scripts/run-tests.sh -v           # verbose
scripts/run-tests.sh --race       # with the race detector
scripts/run-tests.sh -run TestFoo # extra args pass through to `go test`
scripts/run-tests.sh --help
```

Or call Go directly:

```bash
go vet ./...
go test ./...                     # all unit tests; never touches the network
```

Unit tests isolate on-disk state to a temp `HOME`, so they never read or write
your real `~/.config/todo-ui`. Both `scripts/run-tests.sh` and
`scripts/build-local.sh` run `go vet` + these for you.

### Live Todoist API check (online, opt-in)

The endpoints todo-ui depends on (token validation, full sync, filter,
completed-tasks fetch, and the item add/complete/uncomplete/delete commands) are
the part most likely to break when Todoist changes its API. They live behind the
`integration` build tag, so `go test ./...` never runs them. Run them explicitly:

```bash
scripts/todoist-api-test.sh                            # read-only checks
TODOUI_INTEGRATION_WRITE=1 scripts/todoist-api-test.sh # + create→complete→reopen→delete round-trip
```

The token is read the same way the app reads it: `$TODOIST_API_TOKEN`, else
`~/.config/todoui/config.json`, else `~/.config/todoist/config.json`. Without a
token the checks **skip** rather than fail. Under the hood this is just:

```bash
go test -tags integration -run Integration -v -count=1 .
```

## Versioning

todo-ui follows a **Maven-style snapshot** scheme. The single source of truth is
`var version` in `main.go`.

- **Between releases**, `var version` carries a `-dev` suffix for the *upcoming*
  version — e.g. after `v0.2.1` ships it reads `v0.2.2-dev`. Any build off the
  branch therefore self-identifies as a development build:

  ```bash
  ./todo-ui --version     # → todo-ui v0.2.2-dev
  ```

  The `-dev` suffix is a SemVer pre-release identifier, so it sorts *below* the
  clean version (`v0.2.2-dev` < `v0.2.2`) — exactly like Maven's `-SNAPSHOT`.

- **A release** strips the suffix to a clean `vX.Y.Z`, commits and tags that
  (the git tag and GitHub release are *always* clean — never `-dev`), then
  bumps `var version` to the **next** `-dev` snapshot and commits that. So every
  release produces two commits:

  ```
  main.go = v0.2.2-dev  →  release  →  "Bump version to v0.2.2"   (tagged v0.2.2)
                                     →  "Start v0.2.3-dev development"
  ```

Don't edit `var version` by hand — `scripts/release.sh` owns it. Release builds
also stamp it via `-ldflags "-X main.version=vX.Y.Z"`.

## Release

One command does the whole cycle — version, tests, sign + notarize, build, and
(after a confirmation) tag + publish:

```bash
scripts/release.sh                # auto-pick the next version, build, prompt, publish
scripts/release.sh v0.3.0         # release a specific version
scripts/release.sh --no-publish   # build into dist/ only; push nothing
scripts/release.sh --tag-only     # just create & push the tag (no build)
scripts/release.sh --skip-mac     # Linux/Windows only (skip sign/notarize)
```

What a full run does:

1. Preflight: clean tree, `gh` auth, fetch tags.
2. Resolve the clean release version (stripping `main.go`'s `-dev`); bump +
   commit `main.go` if needed.
3. `go test ./...`, then the live Todoist API guard **if a token is present**
   (skippable with `SKIP_INTEGRATION=1`; never a blocker when no token exists).
4. Build `dist/`: macOS arm64+amd64 (signed + notarized), Linux amd64+arm64
   (`.tar.gz`), Windows amd64 (`.zip`), plus `SHA256SUMS.txt`.
5. Prompt `Proceed? [y/N]` — **nothing is pushed before this**.
6. Tag (clean `vX.Y.Z`), push branch + tag, `gh release create`.
7. Rebuild the local `./todo-ui` at the released version, then bump `main.go` to
   the next `-dev` snapshot, commit, and push.

Nothing leaves your machine before the confirmation (notarization uploads to
Apple during the build — that's required). See [`RELEASING.md`](RELEASING.md)
for flags, version-selection rules, signing-identity handling, and how to verify
a published artifact.

## Repo layout (quick orientation)

The root is a thin entry point; the app is a single package under
`internal/todoui/` (Go keeps a package's source and `_test.go` files together,
so the tests live there too).

- `main.go` (root) — `package main`; just calls `internal/todoui.Main()`.
- `internal/todoui/main.go` — model, update/key handling, view dispatch, `Main()`.
- `internal/todoui/version.go` — the build-time `version` var, stamped via
  `-ldflags "-X github.com/grassbl8d/todo-ui/internal/todoui.version=…"`.
- `internal/todoui/sync.go` — Todoist Sync API client, token handling, quick-add parsing.
- `internal/todoui/filter.go` — filter expressions and natural-language date parsing.
- `internal/todoui/ideas.go`, `internal/todoui/mindmap_view.go` — 💡 ideas and 🗺 mind map.
- `internal/todoui/*_test.go` — unit tests; `integration_test.go` is the live-API
  suite (tagged `integration`, run against `./internal/todoui`).
- `scripts/` — `build-local.sh`, `run-tests.sh`, `todoist-api-test.sh`,
  `release.sh`, `sign-notarize-macos.sh`.
