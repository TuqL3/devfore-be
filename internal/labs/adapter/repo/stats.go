package repo

import (
	"context"
	"time"

	"github.com/devforge/be/internal/labs/domain"
)

// statsWindow is how far back "active" and "this week" reach. A literal in the
// SQL rather than a parameter: the screen has no control for it, and a window
// nobody can change is one less thing for the two queries to disagree on.
const statsWindow = "7 days"

// RunningSessions lists every live container, oldest first. Oldest rather than
// newest: the one that has been up longest is the one most likely to be
// abandoned, and that is what an admin opens this list to find.
//
// Not filtered by expires_at. A session past its deadline that the reaper has
// not swept yet is exactly the row worth seeing here.
func (r *GradeRepo) RunningSessions(ctx context.Context) ([]domain.RunningSession, error) {
	rows := []struct {
		ID           string
		UserID       int64
		Username     string
		LabTitle     string
		StartedAt    time.Time
		ExpiresAt    time.Time
		HasContainer bool
	}{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT s.id, s.user_id, u.username, l.title AS lab_title,
		        s.started_at, s.expires_at,
		        s.container_id <> '' AS has_container
		   FROM lab_sessions s
		   JOIN users u ON u.id = s.user_id
		   JOIN labs  l ON l.id = s.lab_id
		  WHERE s.status = 'running'
		  ORDER BY s.started_at`,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.RunningSession, len(rows))
	for i, row := range rows {
		out[i] = domain.RunningSession(row)
	}
	return out, nil
}

// Stats reads the admin overview. Two queries, not one: the totals are a single
// row of subselects and the per-lab table is a grouped join, and folding the
// first into the second would repeat every total on every lab row.
func (r *GradeRepo) Stats(ctx context.Context) (*domain.Stats, error) {
	var head struct {
		Students       int
		ActiveStudents int
		Courses        int
		Published      int
		Sessions       int
		SessionsWeek   int
		Running        int
		Submitted      int
	}
	if err := r.db.WithContext(ctx).Raw(
		`SELECT
		   (SELECT count(*) FROM users)                            AS students,
		   (SELECT count(DISTINCT user_id) FROM lab_sessions
		     WHERE started_at > now() - ?::interval)               AS active_students,
		   (SELECT count(*) FROM courses)                          AS courses,
		   (SELECT count(*) FROM courses WHERE status = 'published') AS published,
		   (SELECT count(*) FROM lab_sessions)                     AS sessions,
		   (SELECT count(*) FROM lab_sessions
		     WHERE started_at > now() - ?::interval)               AS sessions_week,
		   (SELECT count(*) FROM lab_sessions WHERE status = 'running')   AS running,
		   (SELECT count(*) FROM lab_sessions WHERE status = 'submitted') AS submitted`,
		statsWindow, statsWindow,
	).Scan(&head).Error; err != nil {
		return nil, err
	}

	// Left joins all the way down, so a lab nobody has opened is still a row.
	// A lab missing from this table would read as "no such lab" rather than as
	// the thing it is: content that is not being used.
	//
	// count(DISTINCT s.id) rather than count(*): the join to lab_answers
	// multiplies each session by its answers, and counting rows would report a
	// ten-question lab as ten times as popular as a one-question one.
	rows := []struct {
		LabID       int64
		LabTitle    string
		CourseTitle string
		Sessions    int
		Submitted   int
		Answered    int
		Retried     int
	}{}
	if err := r.db.WithContext(ctx).Raw(
		`SELECT l.id AS lab_id, l.title AS lab_title,
		        COALESCE(c.title, '') AS course_title,
		        count(DISTINCT s.id)                                        AS sessions,
		        count(DISTINCT s.id) FILTER (WHERE s.status = 'submitted')  AS submitted,
		        count(a.task_id) FILTER (WHERE a.attempts IS NOT NULL)      AS answered,
		        count(a.task_id) FILTER (WHERE a.passed AND a.attempts > 1) AS retried
		   FROM labs l
		   -- LEFT: a drill has no course, and dropping drills out of the admin
		   -- dashboard would hide exactly the labs somebody is watching.
		   LEFT JOIN courses c  ON c.id = l.course_id
		   LEFT JOIN lab_sessions s ON s.lab_id = l.id
		   LEFT JOIN lab_answers a  ON a.session_id = s.id
		  GROUP BY l.id, l.title, c.title
		  ORDER BY count(DISTINCT s.id) DESC, l.title`,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := domain.Stats{
		Students:       head.Students,
		ActiveStudents: head.ActiveStudents,
		Courses:        head.Courses,
		Published:      head.Published,
		Sessions:       head.Sessions,
		SessionsWeek:   head.SessionsWeek,
		Running:        head.Running,
		Submitted:      head.Submitted,
		Labs:           make([]domain.LabStat, len(rows)),
	}
	for i, row := range rows {
		out.Labs[i] = domain.LabStat{
			LabID:       row.LabID,
			LabTitle:    row.LabTitle,
			CourseTitle: row.CourseTitle,
			Sessions:    row.Sessions,
			Submitted:   row.Submitted,
			Answered:    row.Answered,
			Retried:     row.Retried,
		}
	}
	return &out, nil
}
