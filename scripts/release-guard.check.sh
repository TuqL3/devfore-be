#!/usr/bin/env bash
# Runs scripts/release-guard.sh against the names that matter.
#
# Usage: make check-release-guard   (or: scripts/release-guard.check.sh)
set -uo pipefail
cd "$(dirname "$0")/.."
GUARD=scripts/release-guard.sh
fails=0

accept() {
  if ./"$GUARD" "$1" >/dev/null 2>&1; then
    echo "  ok      accepts $1"
  else
    echo "  FAIL    rejected $1, which is a legal release tag"; fails=$((fails+1))
  fi
}
reject() {
  if ./"$GUARD" "$1" >/dev/null 2>&1; then
    echo "  FAIL    accepted $1 — $2"; fails=$((fails+1))
  else
    echo "  ok      rejects $1"
  fi
}

echo "release-guard:"
accept v1.2.0
accept v0.1.0
accept v10.0.0-rc.1
accept v1.2.0_hotfix

reject ""              "empty"
reject master          "a branch name is not a release"
reject 1.2.0           "no leading v"
reject vX.Y.Z          "v must be followed by a digit"
reject 'v1.0;id'       "command separator"
reject 'v1.0$(id)'     "command substitution"
reject 'v1.0 && id'    "shell operator"
reject 'v1.0/../../etc' "path traversal"
reject 'v1.0`id`'      "backtick substitution"

echo
if [ "$fails" -eq 0 ]; then echo "release-guard: all checks passed"; else echo "release-guard: $fails FAILED"; fi
exit "$fails"
