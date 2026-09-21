#!/usr/bin/env bash
# Refuses a tag that must not become a release.
#
# The tag name is interpolated into a docker tag and, in the deploy job, into a
# command that runs over ssh on the production box. The workflow's `tags: ["v*"]`
# filter is a filter on names, not on shell metacharacters: `v1;rm -rf /` matches
# `v*` perfectly well.
#
# One script rather than the same `case` copied into two jobs, so that the rule
# and the thing that tests the rule cannot drift apart.
#
# Usage: scripts/release-guard.sh v1.2.0
set -euo pipefail

TAG="${1:?usage: release-guard.sh <tag>}"

case "$TAG" in
  # Leading v, then a digit: `vX...`. Then the whole name must be spelled with
  # characters that are legal in a docker tag and inert in a shell — deleting
  # every one of them has to leave nothing behind.
  v[0-9]*) [ -z "${TAG//[A-Za-z0-9._-]/}" ] || { echo "refusing tag: $TAG" >&2; exit 1; } ;;
  *) echo "refusing tag: $TAG" >&2; exit 1 ;;
esac
