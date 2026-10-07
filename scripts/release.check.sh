#!/usr/bin/env bash
# Pins scripts/release.sh: which releases are refused, and that an accepted one
# tags the commits serving on dev — not master's HEAD — in both repos.
#
#   ./scripts/release.check.sh
#
# Throwaway repos with bare "origins" in a temp dir. The real script is copied
# in and run against them; only the ssh is replaced, via RELEASE_DEPLOYED.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t

repo() { # repo <name> → a clone with master and develop, develop one commit ahead
  git init -q --bare "$tmp/$1.git"
  git clone -q "$tmp/$1.git" "$tmp/$1" 2>/dev/null
  git -C "$tmp/$1" checkout -q -b master
  git -C "$tmp/$1" commit -q --allow-empty -m base
  git -C "$tmp/$1" push -q origin master
  git -C "$tmp/$1" checkout -q -b develop
  git -C "$tmp/$1" commit -q --allow-empty -m feature
  git -C "$tmp/$1" push -q origin develop
}
repo be; repo fe
mkdir "$tmp/be/scripts"
cp "$here/release.sh" "$here/release-guard.sh" "$tmp/be/scripts/"
be=$(git -C "$tmp/be" rev-parse develop)
fe=$(git -C "$tmp/fe" rev-parse develop)
on_dev="tag=develop
be=$be
fe=$fe"

fails=0
run() { FE="$tmp/fe" RELEASE_DEPLOYED="$2" "$tmp/be/scripts/release.sh" "$1" >/dev/null 2>&1; }
refuses() { # refuses <case> <version> <deployed>
  if run "$2" "$3"; then echo "FAIL  $1: accepted"; fails=$((fails + 1)); else echo "ok    $1"; fi
}
tag_at() { git -C "$tmp/$1.git" rev-parse -q --verify "refs/tags/$2^{commit}" 2>/dev/null || true; }

refuses "bad version name"                  "1.0.0"   "$on_dev"
refuses "nothing recorded on dev"           "v1.0.0"  ""
refuses "dev commit not merged into master" "v1.0.0"  "$on_dev"
[ -z "$(tag_at be v1.0.0)$(tag_at fe v1.0.0)" ] && echo "ok    refused release left no tag" \
  || { echo "FAIL  refused release left a tag"; fails=$((fails + 1)); }

# Merge develop into master the way a PR does — a merge commit — so master's
# HEAD is NOT the commit dev ran. The tag must still land on the dev commit.
for r in be fe; do
  git -C "$tmp/$r" checkout -q master
  git -C "$tmp/$r" merge -q --no-ff develop -m "merge develop"
  git -C "$tmp/$r" push -q origin master
done

if run "v1.0.0" "$on_dev" && [ "$(tag_at be v1.0.0)" = "$be" ] && [ "$(tag_at fe v1.0.0)" = "$fe" ]; then
  echo "ok    release tags the dev commits in both repos"
else
  echo "FAIL  release did not tag the dev commits"; fails=$((fails + 1))
fi
refuses "version already released"         "v1.0.0"  "$on_dev"

[ "$fails" = 0 ] && echo "release: all checks passed" || { echo "release: $fails check(s) failed"; exit 1; }
