#!/usr/bin/env bash
#
# Run the live Todoist API integration guard. These tests hit the real Todoist
# server to verify the endpoints todo-ui depends on still work — the integration
# most likely to break. They are behind the `integration` build tag, so the
# normal `go test ./...` never runs them.
#
# Usage:
#   scripts/integration-test.sh                 # read-only checks
#   TODOUI_INTEGRATION_WRITE=1 scripts/integration-test.sh   # + create/delete round-trip
#
# A token is read from $TODOIST_API_TOKEN or ~/.config/todoui/config.json
# (same as the app). Without one, the tests skip rather than fail.
#
set -euo pipefail

cd "$(dirname "$0")/.."

if [ -z "${TODOIST_API_TOKEN:-}" ] && [ ! -f "$HOME/.config/todoui/config.json" ] \
   && [ ! -f "$HOME/.config/todoist/config.json" ]; then
  echo "integration: no Todoist token found." >&2
  echo "  set TODOIST_API_TOKEN=… or log in via the app first." >&2
  exit 1
fi

echo "==> go test -tags integration -run Integration"
go test -tags integration -run Integration -v -count=1 .
