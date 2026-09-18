#!/usr/bin/env bash
# Deploy. Called by CD over SSH and by hand for the first release.
#
#   IMAGE_TAG=v1.2.0 ./deploy/up.sh
#
# Nothing is built here any more. Images come from GHCR, built once in Actions,
# scanned there, and promoted by tag (INFRA.md §8): the bits that went through CI
# and staging are the bits that serve production. A rebuild on the box would be
# a different artifact wearing the same version number.
#
# The direct payoff is rollback. It used to mean recompiling the previous commit
# while the broken one kept serving; it now means starting an image that is
# already on the disk, which is seconds.
set -euo pipefail

cd "$(dirname "$0")/.."

: "${IMAGE_TAG:?usage: IMAGE_TAG=v1.2.0 ./deploy/up.sh}"

# One release at a time. CD serialises itself through `concurrency: deploy-prod`
# in ci.yml, but that group is scoped to one repository and knows nothing about
# a person on the box — and the runbook (INFRA.md §13.4) has someone run this
# script by hand.
#
# -n, not a wait: a second deploy queued behind a first is a deploy nobody is
# watching any more. Fail loudly and let whoever pushed decide.
#
# The lock is a read-only fd on this checkout's own directory, not a lock file.
# A file under /run/lock would be created by whoever deployed first: one `sudo
# ./deploy/up.sh` leaves it root-owned at 0644 and every CD run afterwards dies
# on `Permission denied` — the same shape of permanently-wedged CD this script
# already had once. A directory both users can read has no such state, leaves
# nothing behind, and scopes the lock to the deployment rather than the host.
exec 9<"$PWD"
flock -n 9 || { echo "another deploy already holds $PWD — refusing to run two at once" >&2; exit 1; }

COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.prod.yml)
NETWORK=devforge-be_default
LABS=(linux git docker net)
# The tag that is currently serving, written only after a release goes healthy.
# This is the rollback target, and a file rather than `git rev-parse` because
# the thing being rolled back is an image now, not a checkout.
TAG_FILE=.image-tag

[ -f .env ] || { echo "no .env — copy .env.prod.example and fill it in"; exit 1; }
set -a; . ./.env; set +a
: "${DOMAIN:?}" "${DATABASE_URL:?}" "${IMAGE_REPO:?}"

PREV=""
[ -f "$TAG_FILE" ] && PREV=$(cat "$TAG_FILE")

# Lab images are pulled and then renamed to the names the database already
# holds: lab_images rows say `devforge/linux:latest` and the API asks the Docker
# API for exactly that (scripts/seed.sql). Retagging locally is instant and
# keeps that contract, so shipping scanned images costs no migration.
pull() {
  local tag=$1
  IMAGE_TAG=$tag "${COMPOSE[@]}" pull --quiet api web
  for img in "${LABS[@]}"; do
    docker pull --quiet "$IMAGE_REPO/devforge-lab-$img:$tag" >/dev/null
    docker tag "$IMAGE_REPO/devforge-lab-$img:$tag" "devforge/$img:latest"
  done
}

#   release <tag>                 # forward: migrate, then start
#   release <tag> skip-migrate    # rollback: start only
#
# skip-migrate exists because the rollback path calls this a second time and
# must NOT migrate again: the version that just failed may have already applied
# a migration, and `migrate up` would then be run against a checkout whose
# migrations/ directory is the one that applied it. Nothing is migrated down
# either — a down migration during an incident is how a rollback becomes data
# loss. Rolling only the image back is safe because migrations are required to
# be backward compatible (INFRA.md §8).
release() {
  local tag=$1
  if [ "${2:-}" != skip-migrate ]; then
    # Postgres has to be up before migrate can reach it, and migrate runs on the
    # compose network because DATABASE_URL names the service, not localhost.
    # This lands before the new api container starts: a migration is required to
    # be backward compatible, the binary is not required to run against the old
    # schema.
    IMAGE_TAG=$tag "${COMPOSE[@]}" up -d --wait postgres redis
    docker run --rm --network "$NETWORK" -v "$PWD/migrations:/migrations" \
      migrate/migrate:v4.18.1 -path=/migrations -database "$DATABASE_URL" up
  fi
  # --wait is the healthcheck loop: compose blocks until every service with one
  # reports healthy and exits non-zero if any gives up. The api's healthcheck is
  # the binary probing its own /readyz (docker-compose.prod.yml), so this covers
  # database and redis reachability, not just "the process started".
  IMAGE_TAG=$tag "${COMPOSE[@]}" up -d --wait --wait-timeout 120
}

echo "==> releasing $IMAGE_TAG (currently ${PREV:-none})"
pull "$IMAGE_TAG"

if release "$IMAGE_TAG"; then
  echo "$IMAGE_TAG" > "$TAG_FILE"
  echo "==> healthy on $IMAGE_TAG"
  docker image prune -f >/dev/null
  exit 0
fi

echo "==> $IMAGE_TAG never came up healthy" >&2
IMAGE_TAG="$IMAGE_TAG" "${COMPOSE[@]}" logs --tail 50 api >&2 || true

if [ -z "$PREV" ]; then
  echo "==> NO PREVIOUS TAG TO ROLL BACK TO — this is the first release on this box." >&2
  echo "    The stack is left as it is; look at the logs above." >&2
  exit 1
fi

echo "==> rolling back to $PREV" >&2
# Only the image goes back. The checkout stays on the failed tag, which is
# harmless — CD force-checks-out the next tag rather than pulling, so there is
# no branch state to get stuck on — but it does mean the nginx config and the
# migrations on disk are the new ones. Both are required to be backward
# compatible with the previous image for exactly this reason.
pull "$PREV"
if release "$PREV" skip-migrate; then
  echo "==> rolled back and healthy on $PREV" >&2
else
  echo "==> ROLLBACK ALSO UNHEALTHY — look at the box" >&2
fi
exit 1
