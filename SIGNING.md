# Signing & notarizing the macOS builds

Unsigned binaries downloaded from a GitHub release are quarantined by macOS, so on
another Mac they fail with *"cannot be opened because the developer cannot be verified"*
(or *"is damaged"*). The fix is to **sign with a Developer ID Application certificate** and
**notarize** with Apple. You need your Apple Developer account ($99/yr) — which you have.

## One-time setup

### 1. Create a "Developer ID Application" certificate

Easiest via Xcode:

1. Xcode → **Settings → Accounts**, sign in, select your team.
2. **Manage Certificates… → + → Developer ID Application**. It's added to your login keychain.

Or via the web: create a CSR in **Keychain Access → Certificate Assistant → Request a
Certificate from a Certificate Authority**, upload it at
<https://developer.apple.com/account/resources/certificates> (type *Developer ID
Application*), download the `.cer`, and double-click to install.

Verify it's installed:

```bash
security find-identity -v -p codesigning
# look for:  "Developer ID Application: Your Name (TEAMID)"
```

### 2. Store notarization credentials

Create an **app-specific password** at <https://appleid.apple.com> (Sign-In & Security →
App-Specific Passwords), then:

```bash
xcrun notarytool store-credentials todoui-notary \
  --apple-id "you@example.com" \
  --team-id "TEAMID" \
  --password "xxxx-xxxx-xxxx-xxxx"   # the app-specific password
```

(`TEAMID` is the 10-character ID in parentheses from step 1, also shown at
developer.apple.com → Membership.)

## Build signed + notarized release artifacts

```bash
SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" \
NOTARY_PROFILE="todoui-notary" \
VERSION="v0.1.2" \
scripts/sign-notarize-macos.sh
```

This produces `dist/todo-ui_<version>_darwin_arm64.zip` and `…_amd64.zip` with signed,
notarized binaries. Upload those to the GitHub release in place of the plain `.tar.gz`
macOS archives (the Linux/Windows archives are unaffected — Windows uses a different
signing scheme, and Linux needs none).

## Cutting a release

This file covers the one-time cert/notary setup. For the actual release workflow
— `scripts/release.sh`, auto-versioning, `--tag-only`, and the flags — see
**[`RELEASING.md`](RELEASING.md)**. The short version:

```bash
scripts/release.sh        # auto-version, build, sign, notarize, prompt, publish
```

(with `SIGN_IDENTITY` exported in your shell, as `RELEASING.md` describes).

## Notes & caveats

- **Bare CLI binaries can't be *stapled*** (`stapler` only supports `.app`, `.dmg`,
  `.pkg`). Notarization is still registered with Apple, so Gatekeeper accepts the binary
  via an **online check on first run**. If you want offline first-run too, ship a signed +
  notarized + stapled **`.pkg`** (it can install `todo-ui` into `/usr/local/bin`) — ask and
  I'll add a `build-pkg` script.
- **Hardened runtime** (`--options runtime`) is required for notarization; the script sets
  it. Pure-Go binaries need no extra entitlements.
- Verify a finished artifact:
  ```bash
  unzip todo-ui_v0.1.2_darwin_arm64.zip && spctl -a -vvv -t install ./todo-ui_v0.1.2_darwin_arm64/todo-ui
  codesign --verify --strict --verbose=2 ./.../todo-ui
  ```
- **CI option:** the same steps run in GitHub Actions if you store the cert (base64) and
  notary credentials as repository secrets. Ask and I'll add a `release.yml` workflow.
