#!/usr/bin/env bash
# Cut a production release by promoting what the dev box is serving.
#
#   make release v=v1.3.0        # DEV_SSH=devforge@<dev host> in .env or the environment
#
# Nothing is built for a release (INFRA.md §8 "CI/CD"). Every push to develop
# builds devforge-{api,web,lab-*}:dev-<sha> once and deploys it to dev; after a
# healthy deploy, deploy/up.sh writes which be and fe commit are serving to
# .deployed on the box. This script reads that file and tags exactly those two
# commits — not master's HEAD — with the version in both repos. The tag push is
# what makes CI copy dev-<sha> to :<version> and deploy production, so the bits
# that ran on dev are the bits that ship.
#
# RELEASE_DEPLOYED replaces the ssh, for scripts/release.check.sh only.
set -euo pipefail

cd "$(dirname "$0")/.."

V="${1:?usage: scripts/release.sh vX.Y.Z}"
FE="${FE:?no frontend clone next to this one — set FE=<path to devforge-fe>}"

# Same guard CI runs on the tag, so a name refused here is refused there.
./scripts/release-guard.sh "$V"

deployed="${RELEASE_DEPLOYED-$(ssh -o BatchMode=yes -o ConnectTimeout=10 \
  "${DEV_SSH:?set DEV_SSH=devforge@<dev host> (INFRA.md §13.11)}" \
  cat /opt/devforge/devforge-be/.deployed)}"
be=$(sed -n 's/^be=//p' <<<"$deployed")
fe=$(sed -n 's/^fe=//p' <<<"$deployed")

# Absent means dev is mid-deploy or its last deploy failed; a short value means
# images built before revision labels existed. Either way there is nothing on
# dev that can be named.
for sha in "$be" "$fe"; do
  [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || {
    echo "dev has no healthy, labelled deploy recorded — push to develop and wait for deploy-dev" >&2
    exit 1
  }
done
echo "==> dev is serving be ${be:0:12}, fe ${fe:0:12}"

# Every refusal happens before the first tag, so a refused release leaves
# nothing behind in either repo.
for pair in ".|$be" "$FE|$fe"; do
  d=${pair%%|*} sha=${pair#*|}
  git -C "$d" fetch -q origin master --tags
  # Never move a released tag: the image under it may already be serving, and a
  # repointed name makes rollback a guess. Cut the next patch instead.
  if git -C "$d" rev-parse -q --verify "refs/tags/$V" >/dev/null; then
    echo "$d already has $V — cut the next version instead" >&2
    exit 1
  fi
  # CI refuses the same thing; checked here so the refusal costs no push.
  git -C "$d" merge-base --is-ancestor "$sha" origin/master || {
    echo "$d: ${sha:0:12} (serving on dev) is not on master — merge develop into master first" >&2
    exit 1
  }
done

for pair in ".|$be" "$FE|$fe"; do
  d=${pair%%|*} sha=${pair#*|}
  git -C "$d" tag -a "$V" "$sha" -m "$V"
  git -C "$d" push -q origin "$V"
done
echo "==> tagged $V on both repos; Actions now promotes dev-<sha> to $V and deploys production after approval"
