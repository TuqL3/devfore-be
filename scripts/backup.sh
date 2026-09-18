#!/usr/bin/env bash
# pg_dump -> gzip -> Cloudflare R2. Run from a cron entry on the box:
#
#   15 3 * * * cd /opt/devforge/devforge-be && ./scripts/backup.sh >> /var/log/devforge-backup.log 2>&1
#
# This is the first thing that has to work, not the last. The host is free-tier
# with no SLA and an account that can be closed with no explanation: if that
# happens, the dump on R2 is the only thing left (INFRA.md §9.7).
#
# Retention is an R2 lifecycle rule (delete after 30 days), not code here.
set -euo pipefail

cd "$(dirname "$0")/.."
set -a; . ./.env; set +a

: "${R2_BUCKET:?}" "${R2_ENDPOINT:?}" "${R2_ACCESS_KEY_ID:?}" "${R2_SECRET_ACCESS_KEY:?}"

KEY="pg/devforge-$(date -u +%Y%m%dT%H%M%SZ).sql.gz"
TMP="$(mktemp -t devforge-dump-XXXXXX.sql.gz)"
trap 'rm -f "$TMP"' EXIT

# -T because cron has no tty. Written to a temp file rather than piped straight
# into the uploader: a pipe hides pg_dump's exit code behind the uploader's,
# and an empty-but-successful upload is the failure mode that matters here.
docker compose exec -T postgres \
  pg_dump -U "$DB_USER" -d "$DB_NAME" --no-owner --no-privileges \
  | gzip -9 > "$TMP"

SIZE=$(stat -c %s "$TMP")
# A real dump of this schema is tens of KB minimum; anything under 1 KB is an
# error message that got gzipped.
if [ "$SIZE" -lt 1024 ]; then
  echo "backup: dump is only ${SIZE} bytes, refusing to upload" >&2
  exit 1
fi

docker run --rm \
  -e AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID" \
  -e AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY" \
  -e AWS_DEFAULT_REGION=auto \
  -v "$TMP:/dump.sql.gz:ro" \
  amazon/aws-cli:latest \
  s3 cp /dump.sql.gz "s3://${R2_BUCKET}/${KEY}" --endpoint-url "$R2_ENDPOINT"

echo "backup: uploaded ${KEY} (${SIZE} bytes)"
