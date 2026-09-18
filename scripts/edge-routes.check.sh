#!/usr/bin/env bash
# What this pins: deploy/nginx/devforge.conf routes every path to the right
# place. `nginx -t` only says the file parses — it says nothing about the two
# things that actually break, and both break only in production:
#
#   • /uploads/* falling through to the SPA, which serves index.html with a 200
#     so the image is broken rather than 404 (INFRA.md §8 "Domain & routing")
#   • the crawler blocks landing after the catch-all, so shared links stay blank
#
# It stands up the real edge config against two stub upstreams named `api` and
# `web` and asks it questions over HTTPS. No cluster, no fixtures: containers on
# a throwaway network, torn down on exit.
#
#   ./scripts/edge-routes.check.sh
set -euo pipefail
cd "$(dirname "$0")/.."

NET=devforge-edge-check
IMG=nginx:1.27-alpine
TMP=$(mktemp -d)
BOT='facebookexternalhit/1.1'
fails=0

cleanup() {
  docker rm -f edge-check api web >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT

# Self-signed stand-in for the Cloudflare Origin Certificate. The real one is
# only trusted by Cloudflare, so curl would have to skip verification either
# way — what is under test is routing, not trust.
openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
  -keyout "$TMP/origin.key" -out "$TMP/origin.pem" -subj "/CN=devforge.test" 2>/dev/null

# Each stub answers with its own name and the URI it was asked for, which is
# how a test can tell "reached the API" from "reached the SPA" and catch a
# rewrite that fires with the wrong path.
stub() {
  cat > "$TMP/$1.conf" <<CONF
server {
  listen $2;
  location / { default_type text/plain; return 200 "$1 \$request_uri xff=\$http_x_forwarded_for\n"; }
}
CONF
}
stub api 8080
stub web 80

docker network create "$NET" >/dev/null
docker run -d --rm --name api --network "$NET" \
  -v "$TMP/api.conf:/etc/nginx/conf.d/default.conf:ro" "$IMG" >/dev/null
docker run -d --rm --name web --network "$NET" \
  -v "$TMP/web.conf:/etc/nginx/conf.d/default.conf:ro" "$IMG" >/dev/null
docker run -d --rm --name edge-check --network "$NET" -p 8443:443 -p 8080:80 \
  -v "$PWD/deploy/nginx/devforge.conf:/etc/nginx/conf.d/default.conf:ro" \
  -v "$TMP:/etc/nginx/certs:ro" "$IMG" >/dev/null

# The edge needs its upstreams resolvable before the first request, and the
# stubs need to be listening. Poll rather than sleep a guessed number.
for _ in $(seq 40); do
  curl -sk --max-time 2 https://127.0.0.1:8443/healthz >/dev/null 2>&1 && break
  sleep 0.25
done

# want <description> <expected body substring> <curl args...>
want() {
  local desc=$1 expect=$2; shift 2
  local got
  got=$(curl -sk --max-time 5 "$@" 2>&1 || true)
  if [[ "$got" == *"$expect"* ]]; then
    printf 'ok    %s\n' "$desc"
  else
    printf 'FAIL  %s\n        want substring: %s\n        got: %s\n' "$desc" "$expect" "${got//$'\n'/ }"
    fails=$((fails + 1))
  fi
}

# want_absent <description> <substring that must NOT appear> <curl args...>
want_absent() {
  local desc=$1 forbidden=$2; shift 2
  local got
  got=$(curl -sk --max-time 5 "$@" 2>&1 || true)
  if [[ "$got" != *"$forbidden"* ]]; then
    printf 'ok    %s\n' "$desc"
  else
    printf 'FAIL  %s\n        must not contain: %s\n        got: %s\n' "$desc" "$forbidden" "${got//$'\n'/ }"
    fails=$((fails + 1))
  fi
}

B=https://127.0.0.1:8443

want 'REST goes to the api'            'api /api/ping'                  "$B/api/ping"
want 'uploads go to the api, NOT the SPA' 'api /uploads/abc.png'        "$B/uploads/abc.png"
want 'healthz goes to the api'         'api /healthz'                   "$B/healthz"
want 'readyz goes to the api'          'api /readyz'                    "$B/readyz"
want 'websocket path goes to the api'  'api /ws/terminal/42'            "$B/ws/terminal/42"
want 'anything else goes to the SPA'   'web /war-room'                  "$B/war-room"
want 'a share link is the SPA for a person' 'web /r/abc123'             "$B/r/abc123"
want 'a share link is a preview for a bot'  'api /api/shared-drills/abc123/preview' \
                                                                        -A "$BOT" "$B/r/abc123"
want 'a day link is the SPA for a person'   'web /war-room/day/2026-01-02' \
                                                                        "$B/war-room/day/2026-01-02"
want 'a day link is a preview for a bot'    'api /api/daily-drill/2026-01-02/preview' \
                                                                        -A "$BOT" "$B/war-room/day/2026-01-02"
# The id patterns are narrow because what is behind them renders an image.
# A path that misses them is a normal SPA route, never a rewrite.
want 'a malformed share id never reaches the preview endpoint' 'web /r/has.a.dot' \
                                                                        -A "$BOT" "$B/r/has.a.dot"
want 'a malformed day never reaches the preview endpoint' 'web /war-room/day/2026-1-2' \
                                                                        -A "$BOT" "$B/war-room/day/2026-1-2"
# INFRA.md §9.5: a caller who invents an X-Forwarded-For must not be believed.
# nginx replaces the header, so what the api sees is the peer it actually spoke
# to. Asserted as an absence rather than a value: the peer here is whatever
# address this docker host uses, and pinning that would pin the test to a
# machine rather than to the rule.
want_absent 'a forged X-Forwarded-For is replaced, not passed through' '1.2.3.4' \
                                                                        -H 'X-Forwarded-For: 1.2.3.4' "$B/api/ping"
want 'port 80 redirects to https'      '301 https://'                   -o /dev/null \
                                     -w '%{http_code} %{redirect_url}' "http://127.0.0.1:8080/labs"

if [ "$fails" -gt 0 ]; then
  echo
  echo "$fails route(s) wrong — deploy/nginx/devforge.conf"
  exit 1
fi
echo
echo "all routes correct"
