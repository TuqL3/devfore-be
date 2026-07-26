#!/usr/bin/env bash
# Usage: ./scripts/smoke-auth.sh  [BASE_URL]
set -euo pipefail
BASE="${1:-http://localhost:8080}"
U="smoke_$RANDOM"
PASS="s3cr3tpass"

j() { python3 -c 'import sys,json;print(json.load(sys.stdin)[sys.argv[1]])' "$1"; }
step() { printf '\n\033[36m# %s\033[0m\n' "$1"; }

step "healthz"
curl -sf "$BASE/healthz"; echo

step "readyz (DB)"
curl -sf "$BASE/readyz"; echo

step "register $U"
REG=$(curl -sf -X POST "$BASE/api/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$U\",\"email\":\"$U@test.dev\",\"password\":\"$PASS\"}")
echo "$REG"
ACCESS=$(echo "$REG" | j access_token)
REFRESH=$(echo "$REG" | j refresh_token)

step "me (with access token)"
curl -sf "$BASE/api/me" -H "Authorization: Bearer $ACCESS"; echo

step "me without token → expect 401"
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/me")
echo "status=$code"; [ "$code" = "401" ] || { echo "FAIL: expected 401"; exit 1; }

step "login (by username)"
LOGIN=$(curl -sf -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"login\":\"$U\",\"password\":\"$PASS\"}")
echo "$LOGIN"

step "login wrong password → expect 401"
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"login\":\"$U\",\"password\":\"wrong\"}")
echo "status=$code"; [ "$code" = "401" ] || { echo "FAIL: expected 401"; exit 1; }

step "duplicate register → expect 409"
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$U\",\"email\":\"$U@test.dev\",\"password\":\"$PASS\"}")
echo "status=$code"; [ "$code" = "409" ] || { echo "FAIL: expected 409"; exit 1; }

step "refresh"
NEW=$(curl -sf -X POST "$BASE/api/auth/refresh" \
  -H 'Content-Type: application/json' \
  -d "{\"refresh_token\":\"$REFRESH\"}")
echo "$NEW"
NEW_ACCESS=$(echo "$NEW" | j access_token)
curl -sf "$BASE/api/me" -H "Authorization: Bearer $NEW_ACCESS" >/dev/null && echo "new access token works"

printf '\n\033[32mALL PASS\033[0m\n'
