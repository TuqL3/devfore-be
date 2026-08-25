#!/usr/bin/env bash
# Deploy. Called by CD over SSH and by hand for the first release.
#
#   ./deploy/up.sh
#
# Images are built on this box, not pulled: the GitHub runner is amd64 and
# emulating arm64 costs ten times what four Ampere cores need to compile a
# static Go binary (README §9.2). The trade is that a rollback rebuilds instead
# of retagging, which is why the previous commits are captured up front.
set -euo pipefail

cd "$(dirname "$0")/.."
BE="$PWD"
FE="$PWD/../devforge-fe"
COMPOSE=(docker compose -f docker-compose.yml -f docker-compose.prod.yml)
NETWORK=devforge-be_default

[ -f .env ] || { echo "no .env — copy .env.prod.example and fill it in"; exit 1; }
set -a; . ./.env; set +a
: "${DOMAIN:?}" "${DATABASE_URL:?}"

PREV_BE=$(git -C "$BE" rev-parse HEAD)
PREV_FE=$(git -C "$FE" rev-parse HEAD)

release() {
  "${COMPOSE[@]}" build
  # Lab images are built here too and are not part of compose: the API creates
  # containers from them by name through the Docker API.
  for img in linux git docker net; do
    docker build -q -f "$BE/labs/$img/Dockerfile" -t "devforge/$img:latest" "$BE/labs" >/dev/null
  done
  # Postgres has to be up before migrate can reach it, and migrate runs on the
  # compose network because DATABASE_URL names the service, not localhost.
  "${COMPOSE[@]}" up -d postgres redis
  "${COMPOSE[@]}" exec -T postgres sh -c 'until pg_isready -q; do sleep 1; done'
  docker run --rm --network "$NETWORK" -v "$BE/migrations:/migrations" \
    migrate/migrate:v4.18.1 -path=/migrations -database "$DATABASE_URL" up
  "${COMPOSE[@]}" up -d
}

healthy() {
  local cid deadline
  cid=$("${COMPOSE[@]}" ps -q api)
  [ -n "$cid" ] || return 1
  deadline=$((SECONDS + 120))
  while [ $SECONDS -lt $deadline ]; do
    case "$(docker inspect -f '{{.State.Health.Status}}' "$cid")" in
      healthy) return 0 ;;
      unhealthy) return 1 ;;
    esac
    sleep 3
  done
  return 1
}

echo "==> releasing $(git -C "$BE" rev-parse --short HEAD) / fe $(git -C "$FE" rev-parse --short HEAD)"
release

if healthy; then
  echo "==> healthy"
  docker image prune -f >/dev/null
  exit 0
fi

# Rolling the code back is safe on its own because migrations are required to
# be backward compatible (README §8): the previous binary runs against the
# schema the new one just migrated to. Nothing is migrated down here — a down
# migration during an incident is how a rollback turns into data loss.
echo "==> api never came up healthy, rolling back to $PREV_BE" >&2
"${COMPOSE[@]}" logs --tail 50 api >&2 || true
git -C "$BE" checkout --quiet "$PREV_BE"
git -C "$FE" checkout --quiet "$PREV_FE"
release
healthy && echo "==> rolled back and healthy" || echo "==> ROLLBACK ALSO UNHEALTHY — look at the box" >&2
exit 1
