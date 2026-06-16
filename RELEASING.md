# Releasing todo-ui

`scripts/release.sh` is the whole release cycle in one command: it picks the
version, runs the tests, signs + notarizes the macOS binaries, builds the
Linux/Windows archives, writes `SHA256SUMS`, and — after you confirm — tags,
pushes, and creates the GitHub release with every artifact attached.

For the one-time Apple cert / notary setup it depends on, see
[`SIGNING.md`](SIGNING.md).

## TL;DR

```bash
scripts/release.sh
```

That's it. With `SIGN_IDENTITY` exported in your shell (see below) and no version
given, it auto-selects the next version, builds everything, shows you what it
made, and asks once before anything leaves your machine.

## One-time setup on this Mac

1. Apple Developer ID cert + notary profile exist — see `SIGNING.md`.
2. `SIGN_IDENTITY` is exported in `~/.zshrc` (already added):

   ```bash
   export SIGN_IDENTITY="Developer ID Application: CARLO CAMERINO (JRB94C42J3)"
   ```

   > This keychain has **two** Developer ID certs: `JRB94C42J3` (hotmail,
   > **correct**) and `R3A9AC2668` (gmail, **never use**). Because more than one
   > exists, the script will not auto-guess — the export tells it which to use.
   > After editing `~/.zshrc`, open a new terminal (or `source ~/.zshrc`).

3. `gh` is authenticated (`gh auth status`).

## How the version is chosen

You normally don't pass a version. The script infers it:

- It starts from `var version` in `main.go`, floored at the highest **pushed**
  tag / published release, and skips anything already tagged or released.
- Right now `main.go` is `v0.1.6` and it hasn't been released, so the next
  release is **`v0.1.6`**. After that ships, the next run yields `v0.1.7`.
- When the chosen version differs from `main.go`, the script **bumps
  `var version` and commits that change for you** before tagging.

To force a specific version, pass it explicitly:

```bash
scripts/release.sh 0.2.0      # the leading "v" is optional
scripts/release.sh v0.2.0     # same thing
```

The version must be **`X.Y.Z`** — three numeric parts (e.g. `0.1.6`, `1.2.10`).
The `v` prefix is added for you if you omit it; anything that isn't `X.Y.Z` is
rejected. An explicit version is also refused if it's already tagged or released.

> A stray **local-only** `v0.2.0` tag exists in this repo (never pushed/released).
> The auto-picker ignores unpushed tags when choosing the next version, but it
> will never *reuse* one — so it can't accidentally clobber it. The script prints
> a note when a higher local tag exists. Delete it with `git tag -d v0.2.0` if
> it's cruft.

## What a full run does

```
scripts/release.sh
  ├─ preflight: clean tree · gh auth · fetch tags
  ├─ resolve version (auto or explicit); bump+commit main.go if needed
  ├─ go test ./...
  ├─ build into dist/:
  │     macOS arm64 + amd64  → sign → notarize (uploads to Apple)
  │     linux amd64 + arm64  → .tar.gz
  │     windows amd64        → .zip
  │     SHA256SUMS.txt
  ├─ print artifact list
  └─ "Proceed? [y/N]"   ← nothing pushed before this
        └─ git tag · push branch + tag · gh release create --generate-notes
```

## Modes & flags

| Command | What it does |
|---|---|
| `scripts/release.sh` | Auto-version, full build + sign + notarize, prompt, then publish. |
| `scripts/release.sh v0.2.0` | Same, but for an explicit version. |
| `scripts/release.sh --tag-only` | **Just create & push the version tag — no build, no release.** |
| `scripts/release.sh --tag-only --no-publish` | Create the tag **locally only** (don't push). |
| `scripts/release.sh --no-publish` | Build everything into `dist/`, print the manual publish commands, push nothing. |
| `scripts/release.sh --yes` | Skip the confirmation prompt (unattended). |
| `scripts/release.sh --skip-mac` | Skip macOS sign/notarize; build Linux/Windows only (e.g. before the cert exists). |
| `scripts/release.sh --skip-tests` | Skip the `go test ./...` gate. |

Flags combine, e.g. `scripts/release.sh v0.1.7 --skip-mac --no-publish`.

### `--tag-only`

Use this when you want to *mark* a version without producing or publishing
binaries — e.g. to record a tag now and run the signed build later. It runs
preflight, resolves/bumps the version, creates an annotated tag, and pushes it
(branch + tag). Add `--no-publish` to keep the tag local. No artifacts are built
or uploaded.

## Overrides

| Env var | Default | Purpose |
|---|---|---|
| `SIGN_IDENTITY` | (must be set — two certs) | The Developer ID Application identity to sign with. |
| `NOTARY_PROFILE` | `todoui-notary` | The `notarytool` keychain profile. |

## Safety properties

- **Won't run on a dirty tree** — so the tag and version-bump commit are meaningful.
- **Version ↔ source stay in sync** — the binary's `var version` always matches the tag.
- **No accidental publish** — pushing only happens at the confirmation (or `--yes`).
- **Won't reuse a version** — aborts if the chosen tag/release already exists.
- **Won't sign with the wrong cert** — refuses to guess when multiple certs exist.

## Verifying a published artifact

```bash
unzip todo-ui_v0.1.6_darwin_arm64.zip
spctl -a -vvv -t install ./todo-ui_v0.1.6_darwin_arm64/todo-ui   # should say: accepted
shasum -a 256 -c SHA256SUMS.txt                                  # match the release
```
