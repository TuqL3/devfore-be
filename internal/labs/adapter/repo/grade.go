package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"github.com/devforge/be/internal/labs/domain"
)

type GradeRepo struct{ db *gorm.DB }

func NewGradeRepo(db *gorm.DB) *GradeRepo { return &GradeRepo{db: db} }

// Task reads the check script together with the lab and course it belongs to.
// The lab id travels with it because the caller has to refuse a task that is not
// part of the session's own lab — the session id is the only thing proving which
// container the script may run in.
func (r *GradeRepo) Task(ctx context.Context, taskID int64) (*domain.Task, error) {
	var row struct {
		ID          int64
		LabID       int64
		CourseID    int64
		Points      int
		Kind        string
		CheckScript string
		// The answer key, computed in SQL: the ordinal of every option flagged
		// correct, zero-based to match the list the student is shown.
		CorrectOptions   []byte
		OptionCount      int
		ExpectedCommands string
	}
	res := r.db.WithContext(ctx).Raw(
		`SELECT t.id, t.lab_id, l.course_id, t.points, t.kind, t.check_script,
		        t.expected_commands,
		        COALESCE((
		          SELECT jsonb_agg(o.idx - 1 ORDER BY o.idx)
		            FROM jsonb_array_elements(t.options) WITH ORDINALITY AS o(val, idx)
		           WHERE (o.val->>'correct')::boolean
		        ), '[]'::jsonb) AS correct_options,
		        jsonb_array_length(t.options) AS option_count
		   FROM lab_tasks t
		   JOIN labs l ON l.id = t.lab_id
		  WHERE t.id = ?`, taskID,
	).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrTaskNotFound
	}

	t := domain.Task{
		ID:             row.ID,
		LabID:          row.LabID,
		CourseID:       row.CourseID,
		Points:         row.Points,
		Kind:           row.Kind,
		CheckScript:    row.CheckScript,
		CorrectOptions:   []int{},
		OptionCount:      row.OptionCount,
		ExpectedCommands: row.ExpectedCommands,
	}
	if len(row.CorrectOptions) > 0 {
		if err := json.Unmarshal(row.CorrectOptions, &t.CorrectOptions); err != nil {
			return nil, fmt.Errorf("đọc đáp án nhiệm vụ %d: %w", row.ID, err)
		}
	}
	return &t, nil
}

// LabImage resolves the image a lab is pinned to. No filter on active or on
// course status: this answers the admin trial runner, which has to work on the
// draft course an author is still writing.
func (r *GradeRepo) LabImage(ctx context.Context, labID int64) (string, error) {
	var image string
	res := r.db.WithContext(ctx).Raw(
		`SELECT i.name || ':' || i.tag
		   FROM labs l JOIN lab_images i ON i.id = l.lab_image_id
		  WHERE l.id = ?`, labID,
	).Scan(&image)
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 || image == "" {
		// Either the lab is gone or it has no image yet. The author sees the
		// same fix for both: pick an image on the lab.
		return "", domain.ErrNoImage
	}
	return image, nil
}

// PassedTaskIDs is what the lab screen restores its ticks from after a reload.
// Scoped to the session, not to the user: lab_task_completions says a task was
// passed at some point ever, and using that here marked questions done in a
// brand new attempt — the student saw "next question" on a question this
// attempt has no answer for, and the report had none either.
func (r *GradeRepo) PassedTaskIDs(ctx context.Context, sessionID string) ([]int64, error) {
	ids := []int64{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT task_id FROM lab_answers WHERE session_id = ? AND passed`, sessionID,
	).Scan(&ids).Error
	return ids, err
}

// Record writes what one press of the check button changed. Every branch runs in
// the same transaction: the attempt counter, the completion row and the score are
// three writes describing one event, and a crash between them would leave a
// student with points for a task the table says they never passed.
func (r *GradeRepo) Record(
	ctx context.Context, userID int64, sessionID string, t *domain.Task,
	selected []int, passed bool,
) (domain.Grade, error) {
	out := domain.Grade{Passed: passed}

	// Marshalled outside the transaction so a broken payload fails before any
	// of it is written. Never nil: the column is NOT NULL and a script task
	// legitimately has nothing selected.
	if selected == nil {
		selected = []int{}
	}
	picked, err := json.Marshal(selected)
	if err != nil {
		return domain.Grade{}, fmt.Errorf("ghi đáp án nhiệm vụ %d: %w", t.ID, err)
	}

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// What the report reads. Written on every press, passing or not: a
		// wrong answer left as the student's last one is the answer they gave.
		//
		// attempts is the one thing here the row cannot rebuild from its own
		// contents. Everything else is overwritten by the next press, so a task
		// the student got wrong four times before fixing it looks, at hand-in,
		// exactly like one they got right immediately. COALESCE covers rows
		// written before the column existed: they count from one rather than
		// staying null forever once touched again.
		if err := tx.Exec(
			`INSERT INTO lab_answers (session_id, task_id, selected, passed, attempts)
			 VALUES (?, ?, ?, ?, 1)
			 ON CONFLICT (session_id, task_id) DO UPDATE
			 SET selected = EXCLUDED.selected, passed = EXCLUDED.passed,
			     attempts = COALESCE(lab_answers.attempts, 1) + 1,
			     answered_at = now()`,
			sessionID, t.ID, string(picked), passed,
		).Error; err != nil {
			return err
		}

		// Every press counts as an attempt, passing or not — that is what the
		// number means on the leaderboard. It also creates the row the score
		// update below relies on existing.
		if err := tx.Exec(
			`INSERT INTO course_scores (user_id, course_id, score, attempts)
			 VALUES (?, ?, 0, 1)
			 ON CONFLICT (user_id, course_id) DO UPDATE
			 SET attempts = course_scores.attempts + 1, updated_at = now()`,
			userID, t.CourseID,
		).Error; err != nil {
			return err
		}
		if !passed {
			return nil
		}

		// The conflict is the whole point: a task passed an hour ago writes
		// nothing here, and the score below is left alone.
		res := tx.Exec(
			`INSERT INTO lab_task_completions (user_id, task_id, session_id)
			 VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
			userID, t.ID, sessionID,
		)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		out.PointsAwarded = t.Points

		if err := tx.Exec(
			`UPDATE course_scores SET score = score + ?, updated_at = now()
			  WHERE user_id = ? AND course_id = ?`,
			t.Points, userID, t.CourseID,
		).Error; err != nil {
			return err
		}

		// Whether that was the last task standing. Counting what is left rather
		// than comparing totals means a task added to the lab afterwards reopens
		// it, instead of the lab staying complete against a set that grew.
		var remaining int64
		if err := tx.Raw(
			`SELECT count(*) FROM lab_tasks t
			  WHERE t.lab_id = ?
			    AND NOT EXISTS (
			          SELECT 1 FROM lab_task_completions c
			           WHERE c.task_id = t.id AND c.user_id = ?)`,
			t.LabID, userID,
		).Scan(&remaining).Error; err != nil {
			return err
		}
		if remaining > 0 {
			return nil
		}
		out.LabCompleted = true
		// Reached exactly once per lab: it takes the insert above to have written
		// a row, which only happens for the task that completes the set.
		return tx.Exec(
			`UPDATE course_scores SET labs_completed = labs_completed + 1, updated_at = now()
			  WHERE user_id = ? AND course_id = ?`,
			userID, t.CourseID,
		).Error
	})
	if err != nil {
		return domain.Grade{}, err
	}
	return out, nil
}
