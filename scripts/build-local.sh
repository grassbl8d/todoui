#!/usr/bin/env bash
#
# Build and test todo-ui locally on this machine — no signing, no notarizing,
# no publishing. It vets, runs the unit tests, and produces a native ./todo-ui
# binary you can run right away to try the build on your Mac.
#
# Usage:
#   scripts/build-local.sh              # vet + unit tests + native build
#   scripts/build-local.sh --run        # ...then launch ./todo-ui
#   scripts/build-local.sh --skip-tests # build only (skip vet + tests)
#
# This does NOT hit the Todoist API. To verify the live API endpoints, run
# scripts/todoist-api-test.sh separately (needs a token).
#
set -euo pipefail

cd "$(dirname "$0")/.."

RUN=0
SKIP_TESTS=0
for arg in "$@"; do
  case "$arg" in
    --run)        RUN=1 ;;
    --skip-tests) SKIP_TESTS=1 ;;
    -h|--help)    sed -n '3,13p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown arg: $arg (try --help)" >&2; exit 2 ;;
  esac
done

# Mirror release.sh's ldflags so the local binary reports the same version
# (read from internal/todoui/version.go; falls back to "dev").
VERSION="$(sed -n 's/^var version = "\(.*\)"/\1/p' internal/todoui/version.go)"
VERSION="${VERSION:-dev}"
LD="-s -w -X github.com/grassbl8d/todo-ui/internal/todoui.version=${VERSION}"

if [ "$SKIP_TESTS" -eq 0 ]; then
  echo "==> go vet ./..."
  go vet ./...
  echo "==> go test ./..."
  go test ./...
else
  echo "==> vet + tests skipped (--skip-tests)"
fi

echo "==> building ./todo-ui ($VERSION, native $(go env GOOS)/$(go env GOARCH))"
go build -ldflags "$LD" -o todo-ui .

echo "    built: $(pwd)/todo-ui"
echo "    run:   ./todo-ui"

if [ "$RUN" -eq 1 ]; then
  echo "==> launching ./todo-ui"
  exec ./todo-ui
fi
