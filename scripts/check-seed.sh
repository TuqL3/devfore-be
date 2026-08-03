#!/usr/bin/env bash
# Runs every seeded check_script inside a real lab container.
#
# A check script that is wrong marks a student who did the task correctly as
# having failed, and there is no way to find that out by reading it. So each one
# is run the way the grader runs it — same image, same limits, same busybox
# shell — twice per task:
#
#   1. before the task is done, where it MUST fail (a script that passes on a
#      fresh container passes for everybody, forever)
#   2. after the solution from scripts/seed-solutions.tsv, where it MUST pass
#
# One container per lab, tasks in order_idx order: that is a student working
# through the lab, so a task may lean on what the previous one left behind.
set -uo pipefail

cd "$(dirname "$0")/.."
# shellcheck disable=SC1091
[ -f .env ] && . ./.env

SOLUTIONS=scripts/seed-solutions.tsv
# CHECK_DB points this at a scratch database. A dev database usually has content
# written through the admin UI, where the seed's insert-if-absent quietly does
# nothing — checking it would prove nothing about the file.
PSQL=(docker compose exec -T postgres psql -U "${DB_USER:-devforge}" -d "${CHECK_DB:-${DB_NAME:-devforge}}" -tAF $'\t' -q)

fail=0
skipped=0
checked=0

# -v ON_ERROR_STOP so a typo in the SQL below comes back as a failure rather
# than as an empty result that reads exactly like "nothing wrong here".
query() { "${PSQL[@]}" -v ON_ERROR_STOP=1 -c "$1" || { echo "truy vấn lỗi" >&2; exit 2; }; }

# The scripts are multi-line shell, and a tab-separated read would tear them in
# half. base64 crosses the boundary intact — with its own line breaks stripped,
# because postgres wraps the encoding at 76 columns and those breaks would tear
# the row apart exactly the same way.
tasks_sql="
SELECT l.slug, i.name || ':' || i.tag, t.order_idx, t.title,
       replace(encode(convert_to(t.check_script, 'UTF8'), 'base64'), chr(10), '')
  FROM lab_tasks t
  JOIN labs l ON l.id = t.lab_id
  JOIN lab_images i ON i.id = l.lab_image_id
 WHERE t.kind = 'script' AND btrim(t.check_script) <> ''
 ORDER BY l.order_idx, l.id, t.order_idx"

if ! query 'SELECT 1' >/dev/null 2>&1; then
	echo "không nối được postgres — cần 'make up' trước" >&2
	exit 2
fi

# The kinds a container cannot answer for. A choice with no correct option and a
# command with nothing expected are both tasks no student can ever pass, and
# neither shows up as an error anywhere else.
broken=$(query "
SELECT l.slug || ' [' || t.order_idx || '] ' || t.title || ' — ' || v.why
  FROM lab_tasks t
  JOIN labs l ON l.id = t.lab_id
  JOIN LATERAL (VALUES (CASE
         WHEN t.kind = 'choice'  AND jsonb_array_length(t.options) < 2 THEN 'ít hơn 2 lựa chọn'
         WHEN t.kind = 'choice'  AND NOT EXISTS (
              SELECT 1 FROM jsonb_array_elements(t.options) AS o(val)
               WHERE (o.val->>'correct')::boolean) THEN 'không có đáp án đúng'
         WHEN t.kind = 'command' AND btrim(t.expected_commands) = '' THEN 'không có lệnh mong đợi'
         WHEN t.kind = 'script'  AND btrim(t.check_script) = ''      THEN 'không có check_script'
       END)) AS v(why) ON v.why IS NOT NULL
 ORDER BY l.order_idx, t.order_idx")
if [ -n "$broken" ]; then
	echo "$broken" | sed 's/^/  ✗ /'
	fail=$((fail + $(printf '%s\n' "$broken" | wc -l)))
fi

# Same shape as dockerx.Create: a script that needs a network or a writable root
# has to fail here rather than in front of a student.
start_container() {
	docker run -d --rm --name "$1" \
		--network none --read-only --cap-drop ALL \
		--security-opt no-new-privileges \
		--tmpfs /tmp:rw,noexec,nosuid,size=64m \
		--tmpfs /home/student:rw,exec,nosuid,size=64m,uid=1000,gid=1000 \
		-u 1000:1000 -w /home/student \
		--memory 512m --pids-limit 256 \
		"$2" sleep 900 >/dev/null
}

in_lab() { docker exec -u 1000:1000 -w /home/student "$1" /bin/sh -c "$2" >/dev/null 2>&1; }

solution_for() {
	awk -F'\t' -v lab="$1" -v idx="$2" \
		'$1 == lab && $2 == idx { sub(/^[^\t]*\t[^\t]*\t/, ""); print; exit }' "$SOLUTIONS"
}

cid=""
lab=""
cleanup() { [ -n "$cid" ] && docker rm -f "$cid" >/dev/null 2>&1; }
trap cleanup EXIT

while IFS=$'\t' read -r slug image idx title script_b64; do
	[ -z "${slug:-}" ] && continue
	script=$(printf '%s' "$script_b64" | base64 -d)

	if [ "$slug" != "$lab" ]; then
		cleanup
		lab=$slug
		cid="devforge-seedcheck-$slug"
		docker rm -f "$cid" >/dev/null 2>&1
		if ! start_container "$cid" "$image"; then
			echo "✗ $slug: không khởi động được container từ $image"
			fail=$((fail + 1))
			cid=""
			continue
		fi
		echo "── $slug ($image)"
	fi
	[ -z "$cid" ] && continue

	solution=$(solution_for "$slug" "$idx")
	if [ -z "$solution" ]; then
		echo "  ? [$idx] $title — chưa có lời giải trong $SOLUTIONS"
		skipped=$((skipped + 1))
		continue
	fi

	# A leading '!' says this task is already satisfied by an earlier task's
	# solution, so "must fail first" does not apply to it.
	expect_fail=1
	case "$solution" in
	'!'*)
		expect_fail=0
		solution=${solution#!}
		;;
	esac

	checked=$((checked + 1))
	if [ "$expect_fail" = 1 ] && in_lab "$cid" "$script"; then
		echo "  ✗ [$idx] $title — ĐẬU trên container mới (script chấm được cả người chưa làm)"
		fail=$((fail + 1))
		continue
	fi
	if ! in_lab "$cid" "$solution"; then
		echo "  ✗ [$idx] $title — lời giải chạy lỗi trong container"
		fail=$((fail + 1))
		continue
	fi
	if ! in_lab "$cid" "$script"; then
		echo "  ✗ [$idx] $title — vẫn TRƯỢT sau khi làm đúng"
		fail=$((fail + 1))
		continue
	fi
	echo "  ✓ [$idx] $title"
done < <(query "$tasks_sql")

cleanup
echo
echo "đã kiểm $checked task script, $fail lỗi, $skipped thiếu lời giải"
[ "$fail" = 0 ] && [ "$skipped" = 0 ]
