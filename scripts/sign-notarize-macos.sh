#!/usr/bin/env bash
#
# Sign + notarize the macOS todo-ui binaries so they run on other Macs without
# Gatekeeper warnings. Produces dist/todo-ui_<version>_darwin_<arch>.zip with a
# signed, notarized binary inside.
#
# One-time setup (see SIGNING.md):
#   1. Install a "Developer ID Application" certificate in your login keychain.
#   2. Store a notary profile:
#        xcrun notarytool store-credentials todoui-notary \
#          --apple-id "you@example.com" --team-id "TEAMID" \
#          --password "app-specific-password"
#
# Usage:
#   SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" \
#   NOTARY_PROFILE="todoui-notary" \
#   VERSION="v0.1.2" \
#   scripts/sign-notarize-macos.sh
#
set -euo pipefail

VERSION="${VERSION:-dev}"
NOTARY_PROFILE="${NOTARY_PROFILE:-todoui-notary}"
: "${SIGN_IDENTITY:?Set SIGN_IDENTITY to your 'Developer ID Application: ... (TEAMID)' identity}"

cd "$(dirname "$0")/.."
mkdir -p dist

LD="-s -w -X github.com/grassbl8d/todo-ui/internal/todoui.version=${VERSION}"

sign_one() {
  local arch="$1"
  local name="todo-ui_${VERSION}_darwin_${arch}"
  local dir="dist/${name}"
  echo "==> building ${name}"
  rm -rf "$dir" && mkdir -p "$dir"
  cp README.md LICENSE "$dir"/
  GOOS=darwin GOARCH="$arch" go build -ldflags "$LD" -o "$dir/todo-ui" .

  echo "==> signing (hardened runtime + secure timestamp)"
  codesign --force --options runtime --timestamp \
    --sign "$SIGN_IDENTITY" "$dir/todo-ui"
  codesign --verify --strict --verbose=2 "$dir/todo-ui"

  echo "==> zipping for notarization"
  ( cd dist && ditto -c -k --keepParent "${name}" "${name}.zip" )

  echo "==> notarizing (this uploads the zip to Apple and waits)"
  xcrun notarytool submit "dist/${name}.zip" \
    --keychain-profile "$NOTARY_PROFILE" --wait

  # NOTE: a bare CLI binary cannot be stapled (stapler only supports
  # .app/.dmg/.pkg). Notarization is still registered with Apple, so Gatekeeper
  # accepts it via an online check on first run. For fully offline first-run,
  # ship a .pkg/.dmg instead (see SIGNING.md).
  rm -rf "$dir"
  echo "==> done: dist/${name}.zip"
}

sign_one arm64
sign_one amd64

echo
echo "Signed & notarized: dist/todo-ui_${VERSION}_darwin_arm64.zip, dist/todo-ui_${VERSION}_darwin_amd64.zip"
echo "Verify a download with:  spctl -a -vvv -t install ./todo-ui   (after unzip)"
