package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/devforge/be/internal/labs/domain"
)

// History lists a student's own attempts, newest first. Correct counts the
// answers recorded for that session rather than the user's all-time completions:
// the same lab passed last week must not make this attempt look answered.
func (r *GradeRepo) History(ctx context.Context, userID int64) ([]domain.HistoryRow, error) {
	rows := []struct {
		SessionID  string
		LabTitle   string
		LabSlug    string
		CourseSlug string
		Status     string
		StartedAt  time.Time
		EndedAt    *time.Time
		Correct    int
		Total      int
	}{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT s.id AS session_id, l.title AS lab_title, l.slug AS lab_slug,
		        c.slug AS course_slug, s.status, s.started_at,
		        COALESCE(s.submitted_at, s.ended_at) AS ended_at,
		        (SELECT count(*) FROM lab_answers a
		          WHERE a.session_id = s.id AND a.passed) AS correct,
		        (SELECT count(*) FROM lab_tasks t WHERE t.lab_id = l.id) AS total
		   FROM lab_sessions s
		   JOIN labs l   ON l.id = s.lab_id
		   -- LEFT: a War Room challenge has no course, and an inner join would
		   -- drop every drill out of the history list.
		   LEFT JOIN courses c ON c.id = l.course_id
		  WHERE s.user_id = ?
		  ORDER BY s.started_at DESC`, userID,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]domain.HistoryRow, len(rows))
	for i, row := range rows {
		out[i] = domain.HistoryRow{
			SessionID:  row.SessionID,
			LabTitle:   row.LabTitle,
			LabSlug:    row.LabSlug,
			CourseSlug: row.CourseSlug,
			Status:     domain.Status(row.Status),
			StartedAt:  row.StartedAt,
			EndedAt:    row.EndedAt,
			Correct:    row.Correct,
			Total:      row.Total,
		}
	}
	return out, nil
}

// Report is one attempt in full. The caller has already established that the
// session belongs to the student asking and that it is over — this is where the
// answer key is read, and while a session is running that key is the answers.
func (r *GradeRepo) Report(ctx context.Context, sessionID string) (*domain.Report, error) {
	var head struct {
		LabTitle    string
		LabSlug     string
		CourseSlug  string
		Status      string
		StartedAt   time.Time
		EndedAt     *time.Time
		SubmittedAt *time.Time
		LabID       int64
	}
	res := r.db.WithContext(ctx).Raw(
		`SELECT l.title AS lab_title, l.slug AS lab_slug,
		        COALESCE(c.slug, '') AS course_slug,
		        s.status, s.started_at, s.ended_at, s.submitted_at, l.id AS lab_id
		   FROM lab_sessions s
		   JOIN labs l    ON l.id = s.lab_id
		   LEFT JOIN courses c ON c.id = l.course_id
		  WHERE s.id = ?`, sessionID,
	).Scan(&head)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}

	// Left join: a task the student never pressed check on still belongs in the
	// report, as a question with no answer. Without it the report would quietly
	// be a list of the questions they got round to.
	rows := []struct {
		TaskID     int64
		Title      string
		Kind       string
		Points     int
		Options    []byte
		Correct    []byte
		Selected   []byte
		Passed     *bool
		AnsweredAt *time.Time
		Attempts   *int
	}{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT t.id AS task_id, t.title, t.kind, t.points,
		        COALESCE(
		          (SELECT jsonb_agg(o->'text') FROM jsonb_array_elements(t.options) o),
		          '[]'::jsonb
		        ) AS options,
		        COALESCE((
		          SELECT jsonb_agg(o.idx - 1 ORDER BY o.idx)
		            FROM jsonb_array_elements(t.options) WITH ORDINALITY AS o(val, idx)
		           WHERE (o.val->>'correct')::boolean
		        ), '[]'::jsonb) AS correct,
		        COALESCE(a.selected, '[]'::jsonb) AS selected,
		        a.passed, a.answered_at, a.attempts
		   FROM lab_tasks t
		   LEFT JOIN lab_answers a ON a.task_id = t.id AND a.session_id = ?
		  WHERE t.lab_id = ?
		  ORDER BY t.order_idx, t.id`, sessionID, head.LabID,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	rep := domain.Report{
		SessionID:   sessionID,
		LabTitle:    head.LabTitle,
		LabSlug:     head.LabSlug,
		CourseSlug:  head.CourseSlug,
		Status:      domain.Status(head.Status),
		StartedAt:   head.StartedAt,
		EndedAt:     head.EndedAt,
		SubmittedAt: head.SubmittedAt,
		Total:       len(rows),
		Answers:     make([]domain.ReportAnswer, len(rows)),
	}
	for i, row := range rows {
		a := domain.ReportAnswer{
			TaskID:     row.TaskID,
			Title:      row.Title,
			Kind:       row.Kind,
			Points:     row.Points,
			Options:    []string{},
			Correct:    []int{},
			Selected:   []int{},
			Passed:     row.Passed,
			AnsweredAt: row.AnsweredAt,
			Attempts:   row.Attempts,
		}
		// The key, only for a question that was actually answered. Otherwise
		// starting a lab and handing it straight in is a way to read every
		// answer, and the second attempt scores full marks.
		fields := []struct {
			raw []byte
			dst any
		}{{row.Options, &a.Options}, {row.Selected, &a.Selected}}
		if row.Passed != nil {
			fields = append(fields, struct {
				raw []byte
				dst any
			}{row.Correct, &a.Correct})
		}
		for _, f := range fields {
			if len(f.raw) == 0 {
				continue
			}
			if err := json.Unmarshal(f.raw, f.dst); err != nil {
				return nil, fmt.Errorf("đọc báo cáo nhiệm vụ %d: %w", row.TaskID, err)
			}
		}
		if row.Passed != nil && *row.Passed {
			rep.Correct++
		}
		rep.Answers[i] = a
	}
	return &rep, nil
}

// RemainingTasks counts the lab's tasks this session has not passed. Read from
// lab_answers rather than lab_task_completions: a task passed in some earlier
// attempt is not one this attempt has done.
func (r *GradeRepo) RemainingTasks(
	ctx context.Context, sessionID string, labID int64,
) (int, error) {
	var n int
	err := r.db.WithContext(ctx).Raw(
		`SELECT count(*) FROM lab_tasks t
		  WHERE t.lab_id = ?
		    AND NOT EXISTS (
		          SELECT 1 FROM lab_answers a
		           WHERE a.task_id = t.id AND a.session_id = ? AND a.passed)`,
		labID, sessionID,
	).Scan(&n).Error
	return n, err
}

// Submit freezes the attempt. Only a running session can be handed in, and the
// update says so in its WHERE clause rather than in a read before it: two tabs
// pressing the button together must not both count as the submission.
func (r *GradeRepo) Submit(ctx context.Context, sessionID string) error {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE lab_sessions
		    SET status = 'submitted', submitted_at = now(), ended_at = now()
		  WHERE id = ? AND status = 'running'`, sessionID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotRunning
	}
	return nil
}
