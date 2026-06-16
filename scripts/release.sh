#!/usr/bin/env bash
#
# Cut a todo-ui release from your Mac in one command. See RELEASING.md for the
# full guide. In short:
#
#   scripts/release.sh                 # auto-pick the next version, build, sign,
#                                      # notarize, then prompt before publishing
#   scripts/release.sh v0.1.9          # release a specific version
#   scripts/release.sh --tag-only      # just create & push the version tag (no build)
#   scripts/release.sh --no-publish    # build artifacts into dist/, don't publish
#
# The version is normally inferred: it starts at main.go's `var version` and
# skips anything already tagged/released. Pass an explicit vX.Y.Z to override.
# When the chosen version differs from main.go, the script bumps `var version`
# and commits that change for you.
#
# Nothing is pushed/published until the final confirmation (notarization still
# uploads to Apple during the build — that's required). Flags:
#   --tag-only     only create + push the git tag; skip building & releasing
#   --no-publish   build (or, with --tag-only, tag locally) but never push
#   --yes / -y     skip the confirmation prompt
#   --skip-mac     skip macOS sign/notarize (Linux/Windows only)
#   --skip-tests   skip the `go test ./...` gate
#
# Signing identity: set SIGN_IDENTITY (this keychain has two Developer ID certs,
# so it must be named — see RELEASING.md). NOTARY_PROFILE defaults to todoui-notary.
#
set -euo pipefail

cd "$(dirname "$0")/.."

die() { echo "release: $*" >&2; exit 1; }

# ---- args -----------------------------------------------------------------
VERSION=""
PUBLISH="prompt"   # prompt | yes | no
SKIP_MAC=0
SKIP_TESTS=0
TAG_ONLY=0
for arg in "$@"; do
  case "$arg" in
    --no-publish) PUBLISH="no" ;;
    --yes|-y)     PUBLISH="yes" ;;
    --skip-mac)   SKIP_MAC=1 ;;
    --skip-tests) SKIP_TESTS=1 ;;
    --tag-only)   TAG_ONLY=1 ;;
    -*)           die "unknown flag: $arg" ;;
    *)            [ -z "$VERSION" ] && VERSION="$arg" || die "unexpected argument: $arg" ;;
  esac
done

# An explicit version must be X.Y.Z (three numeric parts). The leading "v" is
# optional on input and normalized in — both `release.sh 0.1.6` and
# `release.sh v0.1.6` are accepted; anything else is rejected.
if [ -n "$VERSION" ]; then
  v_norm="v${VERSION#v}"
  [[ "$v_norm" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] \
    || die "version must be X.Y.Z, e.g. 0.1.6 or v0.1.6 (got '$VERSION')"
  VERSION="$v_norm"
fi

NOTARY_PROFILE="${NOTARY_PROFILE:-todoui-notary}"

# ---- helpers --------------------------------------------------------------
bump_patch() {  # v0.1.6 -> v0.1.7
  local v="${1#v}" M rest mi p
  M="${v%%.*}"; rest="${v#*.}"; mi="${rest%%.*}"; p="${rest##*.}"
  echo "v${M}.${mi}.$((p + 1))"
}

# A version is "taken" if a tag exists locally or on origin, or a GitHub release
# uses it. (Remote checks are best-effort and pass through when offline.)
ver_is_taken() {
  local v="$1"
  git rev-parse -q --verify "refs/tags/$v" >/dev/null 2>&1 && return 0
  git ls-remote --tags origin "refs/tags/$v" 2>/dev/null | grep -q . && return 0
  command -v gh >/dev/null 2>&1 && gh release view "$v" >/dev/null 2>&1 && return 0
  return 1
}

semver_only() { grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' || true; }

SRC_VER="$(grep -E '^var version = ' main.go | sed -E 's/.*"(.*)".*/\1/')"
[ -n "$SRC_VER" ] || die "couldn't read 'var version' from main.go"

# ---- preflight ------------------------------------------------------------
echo "==> preflight"
need() { command -v "$1" >/dev/null 2>&1 || die "required tool not found: $1"; }
need go; need git
[ "$PUBLISH" = "no" ] || need gh
if [ "$TAG_ONLY" -eq 0 ] && [ "$SKIP_MAC" -eq 0 ]; then need codesign; need xcrun; need ditto; fi

# Clean working tree so the tag (and any version-bump commit) is meaningful.
if ! git diff --quiet || ! git diff --cached --quiet; then
  die "working tree has uncommitted changes — commit or stash before releasing"
fi

if [ "$PUBLISH" != "no" ]; then
  gh auth status >/dev/null 2>&1 || die "gh is not authenticated (run: gh auth login)"
fi

# Pull down remote tags so version inference sees them (best-effort, additive).
git fetch --tags --quiet origin 2>/dev/null || true

# ---- resolve version ------------------------------------------------------
if [ -z "$VERSION" ]; then
  # Floor at the highest of: source version, remote tags, published releases.
  # (Local-only tags are deliberately ignored here so stray/unpushed tags don't
  # jump the version — but ver_is_taken still refuses to reuse them.)
  remote_tags="$(git ls-remote --tags origin 2>/dev/null | sed -E 's#.*refs/tags/##' | grep -v '\^' || true)"
  rel_tags=""
  if command -v gh >/dev/null 2>&1; then
    rel_tags="$(gh release list --json tagName -q '.[].tagName' 2>/dev/null || true)"
  fi
  VERSION="$(printf '%s\n%s\n%s\n' "$SRC_VER" "$remote_tags" "$rel_tags" | semver_only | sort -V | tail -1 || true)"
  [ -n "$VERSION" ] || VERSION="$SRC_VER"
  while ver_is_taken "$VERSION"; do VERSION="$(bump_patch "$VERSION")"; done
  echo "    auto-selected version: $VERSION  (source main.go is $SRC_VER)"
else
  ver_is_taken "$VERSION" && die "version $VERSION is already tagged or released"
  echo "    version: $VERSION  (source main.go is $SRC_VER)"
fi

# Warn if a higher local-only tag exists (e.g. a stray/unpushed tag).
highest_local="$(git tag -l 'v*' | semver_only | sort -V | tail -1 || true)"
if [ -n "$highest_local" ] && [ "$(printf '%s\n%s\n' "$VERSION" "$highest_local" | sort -V | tail -1)" = "$highest_local" ] \
   && [ "$highest_local" != "$VERSION" ]; then
  echo "    note: local tag $highest_local is higher than $VERSION but isn't pushed/released; releasing $VERSION."
fi

# ---- bump main.go if needed -----------------------------------------------
if [ "$SRC_VER" != "$VERSION" ]; then
  echo "==> bumping main.go: $SRC_VER -> $VERSION"
  sed -i '' -E "s/^var version = \".*\"/var version = \"$VERSION\"/" main.go
  git add main.go
  git commit -q -m "Bump version to $VERSION"
fi

branch="$(git symbolic-ref --short HEAD)"

# ---- tag-only mode --------------------------------------------------------
if [ "$TAG_ONLY" -eq 1 ]; then
  if [ "$PUBLISH" = "no" ]; then
    git tag -a "$VERSION" -m "$VERSION"
    echo "Created local tag $VERSION (not pushed). Push later with:  git push origin $VERSION"
    exit 0
  fi
  if [ "$PUBLISH" = "prompt" ]; then
    echo
    read -r -p "Create tag $VERSION and push it (with '$branch') to origin? [y/N] " ans
    case "$ans" in y|Y|yes|YES) ;; *) echo "Aborted (no tag created)."; exit 0 ;; esac
  fi
  git tag -a "$VERSION" -m "$VERSION"
  git push origin "$branch"
  git push origin "$VERSION"
  echo "Tagged and pushed $VERSION (no build/release made)."
  exit 0
fi

# ---- resolve signing identity ---------------------------------------------
# Auto-detect ONLY when exactly one Developer ID Application identity exists; if
# several do (this keychain has two), refuse to guess and require SIGN_IDENTITY.
if [ "$SKIP_MAC" -eq 0 ]; then
  if [ -z "${SIGN_IDENTITY:-}" ]; then
    ids="$(security find-identity -v -p codesigning 2>/dev/null \
      | grep 'Developer ID Application' | sed -E 's/.*"(.*)".*/\1/')"
    count="$(printf '%s\n' "$ids" | grep -c . || true)"
    if [ "$count" -eq 0 ]; then
      die "no 'Developer ID Application' identity in keychain.
  Set up the cert (see SIGNING.md), or pass --skip-mac to build Linux/Windows only."
    elif [ "$count" -gt 1 ]; then
      die "multiple 'Developer ID Application' identities found — set SIGN_IDENTITY to pick one:
$(printf '%s\n' "$ids" | sed 's/^/    SIGN_IDENTITY=/')"
    fi
    SIGN_IDENTITY="$ids"
  fi
  echo "    signing identity: $SIGN_IDENTITY"
  echo "    notary profile:   $NOTARY_PROFILE"
fi

# ---- tests ----------------------------------------------------------------
if [ "$SKIP_TESTS" -eq 0 ]; then
  echo "==> go test ./..."
  go test ./...
else
  echo "    (skipping tests)"
fi

# ---- build ----------------------------------------------------------------
LD="-s -w -X main.version=${VERSION}"
# Start from an empty dist/ so the release only contains this version's artifacts.
rm -rf dist && mkdir -p dist

if [ "$SKIP_MAC" -eq 0 ]; then
  echo "==> macOS: sign + notarize (arm64 + amd64)"
  SIGN_IDENTITY="$SIGN_IDENTITY" NOTARY_PROFILE="$NOTARY_PROFILE" VERSION="$VERSION" \
    scripts/sign-notarize-macos.sh
else
  echo "==> macOS: skipped (--skip-mac)"
fi

build_plain() {
  local goos="$1" goarch="$2" kind="$3"
  local name="todo-ui_${VERSION}_${goos}_${goarch}"
  local dir="dist/${name}"
  local bin="todo-ui"; [ "$goos" = windows ] && bin="todo-ui.exe"
  echo "==> building ${name}"
  rm -rf "$dir" && mkdir -p "$dir"
  cp README.md LICENSE "$dir"/
  GOOS="$goos" GOARCH="$goarch" go build -ldflags "$LD" -o "$dir/$bin" .
  case "$kind" in
    tar) ( cd dist && tar -czf "${name}.tar.gz" "${name}" ) ;;
    zip) ( cd dist && ditto -c -k --keepParent "${name}" "${name}.zip" ) ;;
  esac
  rm -rf "$dir"
}

build_plain linux   amd64 tar
build_plain linux   arm64 tar
build_plain windows amd64 zip

echo "==> SHA256SUMS"
( cd dist && shopt -s nullglob && shasum -a 256 *.zip *.tar.gz > SHA256SUMS.txt )

shopt -s nullglob
artifacts=(dist/*.zip dist/*.tar.gz dist/SHA256SUMS.txt)

echo
echo "==> artifacts for $VERSION:"
( cd dist && ls -1 *.zip *.tar.gz SHA256SUMS.txt 2>/dev/null | sed 's/^/    /' )

# ---- publish --------------------------------------------------------------
if [ "$PUBLISH" = "no" ]; then
  echo
  echo "Built artifacts only (--no-publish). To publish later:"
  echo "    git tag -a $VERSION -m $VERSION && git push origin HEAD && git push origin $VERSION"
  echo "    gh release create $VERSION ${artifacts[*]} --title $VERSION --generate-notes"
  exit 0
fi

if [ "$PUBLISH" = "prompt" ]; then
  echo
  echo "This will:  git tag $VERSION  ·  push '$branch' + tag to origin  ·  create GitHub release $VERSION"
  read -r -p "Proceed? [y/N] " ans
  case "$ans" in y|Y|yes|YES) ;; *) echo "Aborted — artifacts are in dist/ (and main.go is bumped locally)."; exit 0 ;; esac
fi

echo "==> tagging and publishing"
git tag -a "$VERSION" -m "$VERSION"
git push origin "$branch"
git push origin "$VERSION"
gh release create "$VERSION" "${artifacts[@]}" --title "$VERSION" --generate-notes

echo
echo "Released: https://github.com/grassbl8d/todo-ui/releases/tag/$VERSION"
