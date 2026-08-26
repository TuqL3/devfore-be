#!/usr/bin/env bash
# Restore drill. Pulls the newest dump from R2 and loads it into a scratch
# database beside the live one, then counts what came back.
#
#   ./scripts/restore.sh              # drill into devforge_restore_check
#   ./scripts/restore.sh --into-live  # the real thing, after losing the box
#
# A backup nobody has restored is a guess. Run the drill after setting the cron
# up, and again whenever a migration changes the schema shape.
set -euo pipefail

cd "$(dirname "$0")/.."
set -a; . ./.env; set +a

: "${R2_BUCKET:?}" "${R2_ENDPOINT:?}" "${R2_ACCESS_KEY_ID:?}" "${R2_SECRET_ACCESS_KEY:?}"

TARGET="${DB_NAME}_restore_check"
DROP_FIRST=1
if [ "${1:-}" = "--into-live" ]; then
  TARGET="$DB_NAME"
  DROP_FIRST=0
  read -rp "This overwrites the live database ${DB_NAME}. Type the name to confirm: " ok
  [ "$ok" = "$DB_NAME" ] || { echo "aborted"; exit 1; }
fi

aws_r2() {
  docker run --rm -i \
    -e AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID" \
    -e AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY" \
    -e AWS_DEFAULT_REGION=auto \
    amazon/aws-cli:latest "$@" --endpoint-url "$R2_ENDPOINT"
}

# Keys are UTC timestamps, so lexical order is chronological order.
LATEST=$(aws_r2 s3 ls "s3://${R2_BUCKET}/pg/" | awk '{print $4}' | sort | tail -1)
[ -n "$LATEST" ] || { echo "restore: no dump in s3://${R2_BUCKET}/pg/" >&2; exit 1; }
echo "restore: newest dump is ${LATEST}"

TMP="$(mktemp -t devforge-restore-XXXXXX.sql.gz)"
trap 'rm -f "$TMP"' EXIT
aws_r2 s3 cp "s3://${R2_BUCKET}/pg/${LATEST}" - > "$TMP"

psql_as() { docker compose exec -T postgres psql -U "$DB_USER" -v ON_ERROR_STOP=1 "$@"; }

if [ "$DROP_FIRST" = 1 ]; then
  psql_as -d postgres -c "DROP DATABASE IF EXISTS ${TARGET}"
  psql_as -d postgres -c "CREATE DATABASE ${TARGET}"
fi

gunzip -c "$TMP" | docker compose exec -T postgres psql -U "$DB_USER" -d "$TARGET" -q

echo "restore: rows recovered into ${TARGET}"
psql_as -d "$TARGET" -c \
  "SELECT 'users' t, count(*) FROM users
   UNION ALL SELECT 'courses', count(*) FROM courses
   UNION ALL SELECT 'labs', count(*) FROM labs
   UNION ALL SELECT 'lab_task_completions', count(*) FROM lab_task_completions"

[ "$DROP_FIRST" = 1 ] && echo "restore: drill only — live ${DB_NAME} untouched. Drop ${TARGET} when done."
