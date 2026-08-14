package repo

import (
	"context"
	"time"

	"github.com/devforge/be/internal/labs/domain"
)

// The queries behind the admin screens.
//
// All of them are read-only aggregates, and all of them answer a question an
// admin would otherwise have to ask the database by hand. They live here rather
// than in the modules that own each table because the questions cross tables the
// modules do not share — "which task is nobody passing" reads tasks, answers and
// sessions at once, and putting a third of it in three places would mean nobody
// could read the whole thought.
//
// None of them paginate. Every result is bounded by something small and real:
// the number of tasks on the platform, the labs, the courses, or an explicit
// LIMIT on a feed. A cursor would be machinery for a list that fits on a screen.

// TaskHealth: see domain.TaskHealth. Sorted so the two ends worth looking at
// come first — never attempted, then lowest pass rate.
func (r *SessionRepo) TaskHealth(ctx context.Context) ([]domain.TaskHealth, error) {
	out := []domain.TaskHealth{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT t.id       AS task_id,
		       l.id       AS lab_id,
		       l.slug     AS lab_slug,
		       l.title    AS lab_title,
		       t.title    AS task_title,
		       t.kind,
		       count(a.*)                                  AS attempts,
		       count(a.*) FILTER (WHERE a.passed)          AS passed,
		       CASE WHEN count(a.*) = 0 THEN 0
		            ELSE (100 * count(a.*) FILTER (WHERE a.passed) / count(a.*))::int
		       END                                         AS pass_rate
		  FROM lab_tasks t
		  JOIN labs l ON l.id = t.lab_id
		  LEFT JOIN lab_answers a ON a.task_id = t.id
		 GROUP BY t.id, l.id, l.slug, l.title, t.title, t.kind
		 -- Postgres nhận alias TRẦN ở ORDER BY nhưng không nhận alias trong biểu
		 -- thức, nên vế "chưa ai làm" phải viết lại bằng hàm gộp.
		 ORDER BY (count(a.*) = 0), pass_rate, attempts DESC`,
	).Scan(&out).Error
	return out, err
}

// LabHealth is one lab and how attempts on it end.
//
// Abandonment is the number worth reading: a lab that people start and never
// hand in is either too long, broken in the middle, or asking for something the
// instructions never mentioned. None of those are visible from a pass rate.
func (r *SessionRepo) LabHealth(ctx context.Context) ([]domain.LabHealth, error) {
	out := []domain.LabHealth{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT l.id    AS lab_id,
		       l.slug  AS lab_slug,
		       l.title AS lab_title,
		       count(s.*)                                            AS starts,
		       count(s.*) FILTER (WHERE s.status = 'submitted')      AS submitted,
		       count(s.*) FILTER (WHERE s.status = 'expired')        AS expired,
		       count(s.*) FILTER (WHERE s.status = 'ended')          AS ended,
		       count(*) FILTER (WHERE s.status = 'running')          AS running,
		       CASE WHEN count(s.*) = 0 THEN 0
		            ELSE (100 * count(s.*) FILTER (WHERE s.status <> 'submitted')
		                      / count(s.*))::int
		       END                                                   AS drop_rate
		  FROM labs l
		  LEFT JOIN lab_sessions s ON s.lab_id = l.id
		 GROUP BY l.id, l.slug, l.title
		 ORDER BY (count(s.*) = 0), drop_rate DESC, starts DESC`,
	).Scan(&out).Error
	return out, err
}

// IncidentHealth is one War Room scenario and whether anybody beats it.
//
// A scenario nobody has ever recovered is either genuinely brutal or quietly
// impossible — a break script that leaves the service unfixable looks exactly
// like a hard puzzle from the outside, and only this number tells them apart.
func (r *SessionRepo) IncidentHealth(ctx context.Context) ([]domain.IncidentHealth, error) {
	out := []domain.IncidentHealth{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT i.id      AS incident_id,
		       i.title   AS incident_title,
		       i.active,
		       l.slug    AS lab_slug,
		       count(s.*)                                       AS attempts,
		       count(*) FILTER (WHERE ok.recovered)             AS solved,
		       COALESCE(min(ok.downtime) FILTER (WHERE ok.recovered), 0)::int AS best_seconds
		  FROM lab_incidents i
		  JOIN labs l ON l.id = i.lab_id
		  LEFT JOIN lab_sessions s ON s.incident_id = i.id
		  LEFT JOIN LATERAL (
		      SELECT (tk.total > 0 AND ans.done = tk.total) AS recovered,
		             COALESCE(EXTRACT(EPOCH FROM (ans.last_at - s.started_at)), 0) AS downtime
		        FROM (SELECT max(a.answered_at) AS last_at,
		                     count(*) FILTER (WHERE a.passed) AS done
		                FROM lab_answers a WHERE a.session_id = s.id) ans,
		             (SELECT count(*) AS total FROM lab_tasks t WHERE t.lab_id = s.lab_id) tk
		  ) ok ON true
		 GROUP BY i.id, i.title, i.active, l.slug
		 ORDER BY (count(*) FILTER (WHERE ok.recovered) = 0) DESC, attempts DESC`,
	).Scan(&out).Error
	return out, err
}

// CourseHealth is the funnel: signed up, actually opened a lab, finished one.
//
// Enrolment on its own says nothing — it is one click on a page somebody may
// have closed straight after. The gap between the three columns is the number
// that says whether the material is being used.
func (r *SessionRepo) CourseHealth(ctx context.Context) ([]domain.CourseHealth, error) {
	out := []domain.CourseHealth{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.id    AS course_id,
		       c.slug  AS course_slug,
		       c.title AS course_title,
		       c.status,
		       (SELECT count(*) FROM enrollments e WHERE e.course_id = c.id) AS enrolled,
		       (SELECT count(DISTINCT s.user_id)
		          FROM lab_sessions s JOIN labs l ON l.id = s.lab_id
		         WHERE l.course_id = c.id)                                   AS started,
		       (SELECT count(DISTINCT s.user_id)
		          FROM lab_sessions s JOIN labs l ON l.id = s.lab_id
		         WHERE l.course_id = c.id AND s.status = 'submitted')        AS finished
		  FROM courses c
		 ORDER BY enrolled DESC, c.title`,
	).Scan(&out).Error
	return out, err
}

// SharedReports lists every drill report currently public.
//
// The screen that takes one down already existed, but it could only act on a
// link somebody sent in. Nobody could answer "what is public right now", which
// is the question a moderator actually has.
func (r *SessionRepo) SharedReports(ctx context.Context, limit int) ([]domain.SharedReport, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	out := []domain.SharedReport{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.share_token AS token,
		       s.id          AS session_id,
		       u.username    AS player,
		       l.title       AS lab_title,
		       i.title       AS incident_title,
		       s.started_at
		  FROM lab_sessions s
		  JOIN users u          ON u.id = s.user_id
		  JOIN labs  l          ON l.id = s.lab_id
		  LEFT JOIN lab_incidents i ON i.id = s.incident_id
		 WHERE s.share_token IS NOT NULL
		 ORDER BY s.started_at DESC
		 LIMIT ?`, limit,
	).Scan(&out).Error
	return out, err
}

// UserSummary is the header of one person's activity page.
func (r *SessionRepo) UserSummary(ctx context.Context, userID int64) (*domain.UserSummary, error) {
	var row domain.UserSummary
	res := r.db.WithContext(ctx).Raw(`
		SELECT u.id, u.username, u.email, u.status, u.created_at,
		       (SELECT count(*) FROM lab_sessions s WHERE s.user_id = u.id)          AS sessions,
		       (SELECT count(*) FROM lab_sessions s
		         WHERE s.user_id = u.id AND s.status = 'submitted')                  AS submitted,
		       (SELECT count(*) FROM sim_runs r
		          JOIN lab_sessions s ON s.id = r.session_id
		         WHERE s.user_id = u.id)                                             AS sim_runs,
		       (SELECT count(*) FROM chat_messages m
		         WHERE m.user_id = u.id AND m.deleted_at IS NULL)                    AS chat_messages,
		       (SELECT count(*) FROM enrollments e WHERE e.user_id = u.id)           AS enrolments
		  FROM users u WHERE u.id = ?`, userID,
	).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	return &row, nil
}

// UserSessions is what one person has attempted, newest first, with the drill
// half filled in where there was one.
func (r *SessionRepo) UserSessions(ctx context.Context, userID int64, limit int) ([]domain.UserSession, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out := []domain.UserSession{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS session_id, l.slug AS lab_slug, l.title AS lab_title,
		       s.status, s.started_at, s.ended_at,
		       COALESCE(i.title, '') AS incident_title,
		       (SELECT count(*) FROM lab_answers a
		         WHERE a.session_id = s.id AND a.passed)                     AS passed,
		       (SELECT count(*) FROM lab_tasks t WHERE t.lab_id = s.lab_id)  AS total,
		       (s.command_log <> '')                                         AS has_commands
		  FROM lab_sessions s
		  JOIN labs l ON l.id = s.lab_id
		  LEFT JOIN lab_incidents i ON i.id = s.incident_id
		 WHERE s.user_id = ?
		 ORDER BY s.started_at DESC
		 LIMIT ?`, userID, limit,
	).Scan(&out).Error
	return out, err
}

// UserSimRuns is every pipeline this person wrote and ran, with the text of it.
//
// The pipeline is the thing worth reading: it is what they typed, and it says
// what they understood. Truncated in the query rather than on the screen — a
// runaway paste should not travel across the network to be cut off in a browser.
func (r *SessionRepo) UserSimRuns(ctx context.Context, userID int64, limit int) ([]domain.UserSimRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	out := []domain.UserSimRun{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT r.id, r.session_id, r.run_index, r.created_at,
		       left(r.pipeline, 4000) AS pipeline,
		       length(r.pipeline)     AS pipeline_length,
		       COALESCE(l.title, '')  AS lab_title,
		       COALESCE((r.result->>'total_seconds')::int, 0) AS total_seconds
		  FROM sim_runs r
		  JOIN lab_sessions s ON s.id = r.session_id
		  LEFT JOIN labs l    ON l.id = s.lab_id
		 WHERE s.user_id = ?
		 ORDER BY r.created_at DESC
		 LIMIT ?`, userID, limit,
	).Scan(&out).Error
	return out, err
}

// PlatformCounts is the strip of numbers across the top of the live screen.
//
// One query rather than eight round trips: the screen shows them together, they
// are read together, and eight statements is eight chances for the page to be
// half-loaded.
func (r *SessionRepo) PlatformCounts(ctx context.Context, since time.Time) (*domain.PlatformCounts, error) {
	var row domain.PlatformCounts
	err := r.db.WithContext(ctx).Raw(`
		SELECT
		  (SELECT count(*) FROM users)                                           AS users,
		  (SELECT count(*) FROM users WHERE status = 'banned')                   AS banned,
		  (SELECT count(*) FROM users WHERE created_at >= ?)                     AS new_users,
		  (SELECT count(*) FROM lab_sessions WHERE status = 'running'
		     AND container_id <> '')                                             AS running,
		  (SELECT count(*) FROM lab_sessions WHERE started_at >= ?)              AS sessions,
		  (SELECT count(*) FROM lab_sessions
		    WHERE started_at >= ? AND status = 'submitted')                      AS submitted,
		  (SELECT count(*) FROM sim_runs WHERE created_at >= ?)                  AS sim_runs,
		  (SELECT count(*) FROM chat_messages
		    WHERE created_at >= ? AND deleted_at IS NULL)                        AS chat_messages,
		  (SELECT count(*) FROM lab_sessions WHERE share_token IS NOT NULL)      AS shared_reports
		`, since, since, since, since, since,
	).Scan(&row).Error
	return &row, err
}
