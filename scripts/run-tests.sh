#!/usr/bin/env bash
#
# Run the todo-ui unit test suite. These are offline and never touch the Todoist
# API. For the live-API checks, use scripts/todoist-api-test.sh instead.
#
# Usage:
#   scripts/run-tests.sh                 # go vet + go test ./...
#   scripts/run-tests.sh -v              # verbose
#   scripts/run-tests.sh --race          # with the race detector
#   scripts/run-tests.sh --no-vet        # skip the go vet step
#   scripts/run-tests.sh -run TestFoo    # any extra args pass through to go test
#
set -euo pipefail

cd "$(dirname "$0")/.."

VET=1
PASS=()
for a in "$@"; do
  case "$a" in
    --no-vet)  VET=0 ;;
    -v|--verbose) PASS+=(-v) ;;
    --race)    PASS+=(-race) ;;
    -h|--help) sed -n '3,12p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *)         PASS+=("$a") ;;
  esac
done

if [ "$VET" -eq 1 ]; then
  echo "==> go vet ./..."
  go vet ./...
fi

echo "==> go test ./... ${PASS[*]:-}"
if [ "${#PASS[@]}" -gt 0 ]; then
  go test ./... "${PASS[@]}"
else
  go test ./...
fi

echo "==> all unit tests passed"
