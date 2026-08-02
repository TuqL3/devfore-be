package repo

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/devforge/be/internal/labs/domain"
)

// Awarding points is the one write here that a student would notice being wrong,
// and every rule it follows lives in SQL — the conflict clause, the counters, the
// "is anything left" query. That makes a real database the only place it can be
// checked. Skipped rather than failed without one, so `go test ./...` still runs
// on a machine with no postgres.
func TestRecordAwardsEachTaskOnce(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("cần DATABASE_URL trỏ tới postgres đã migrate")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx := context.Background()
	r := NewGradeRepo(db)
	userID, courseID, labID, sessionID, taskIDs := seedGradeFixture(t, db)

	one := &domain.Task{ID: taskIDs[0], LabID: labID, CourseID: courseID, Points: 10}
	two := &domain.Task{ID: taskIDs[1], LabID: labID, CourseID: courseID, Points: 15}

	// A failed attempt counts as an attempt and nothing else.
	if g, err := r.Record(ctx, userID, sessionID, one, []int{1}, false); err != nil {
		t.Fatalf("record fail: %v", err)
	} else if g.PointsAwarded != 0 || g.Passed {
		t.Fatalf("failed check awarded %+v", g)
	}

	g, err := r.Record(ctx, userID, sessionID, one, []int{0}, true)
	if err != nil {
		t.Fatalf("record pass: %v", err)
	}
	if g.PointsAwarded != 10 || g.LabCompleted {
		t.Fatalf("first pass = %+v, want 10 points and lab not complete", g)
	}

	// The retry is the case the primary key exists for.
	if g, err = r.Record(ctx, userID, sessionID, one, []int{0}, true); err != nil {
		t.Fatalf("record repeat: %v", err)
	} else if g.PointsAwarded != 0 {
		t.Fatalf("repeat pass awarded %d points, want 0", g.PointsAwarded)
	}

	if g, err = r.Record(ctx, userID, sessionID, two, nil, true); err != nil {
		t.Fatalf("record second task: %v", err)
	} else if g.PointsAwarded != 15 || !g.LabCompleted {
		t.Fatalf("last task = %+v, want 15 points and lab complete", g)
	}

	var score struct {
		Score         int
		LabsCompleted int
		Attempts      int
	}
	if err := db.Raw(
		`SELECT score, labs_completed, attempts FROM course_scores
		  WHERE user_id = ? AND course_id = ?`, userID, courseID,
	).Scan(&score).Error; err != nil {
		t.Fatalf("read score: %v", err)
	}
	if score.Score != 25 || score.LabsCompleted != 1 || score.Attempts != 4 {
		t.Fatalf("scores = %+v, want score 25, labs 1, attempts 4", score)
	}

	ids, err := r.PassedTaskIDs(ctx, sessionID)
	if err != nil || len(ids) != 2 {
		t.Fatalf("passed ids = %v (err %v), want 2", ids, err)
	}

	// One row per task however many times it was checked, holding the last
	// answer given. Task one was answered [1] wrong, then [0] right, twice.
	var answers []struct {
		TaskID   int64
		Selected string
		Passed   bool
		Attempts int
	}
	if err := db.Raw(
		`SELECT task_id, selected::text AS selected, passed, attempts FROM lab_answers
		  WHERE session_id = ? ORDER BY task_id`, sessionID,
	).Scan(&answers).Error; err != nil {
		t.Fatalf("read answers: %v", err)
	}
	if len(answers) != 2 {
		t.Fatalf("answers = %+v, want one row per task", answers)
	}
	if answers[0].Selected != "[0]" || !answers[0].Passed {
		t.Fatalf("task one answer = %+v, want [0] and passed", answers[0])
	}
	// The count the overwritten row cannot rebuild: three presses on task one,
	// one on task two. Without it the report cannot tell a question fixed on the
	// third go from one answered right immediately.
	if answers[0].Attempts != 3 {
		t.Fatalf("task one attempts = %d, want 3", answers[0].Attempts)
	}
	if answers[1].Selected != "[]" {
		t.Fatalf("task two answer = %+v, want [] for a script task", answers[1])
	}
	if answers[1].Attempts != 1 {
		t.Fatalf("task two attempts = %d, want 1", answers[1].Attempts)
	}

	// A report of the attempt reads back the same two answers, with the key.
	rep, err := r.Report(ctx, sessionID)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if rep.Total != 2 || rep.Correct != 2 {
		t.Fatalf("report = %d/%d, want 2/2", rep.Correct, rep.Total)
	}

	if len(rep.Answers[0].Correct) != 1 || rep.Answers[0].Correct[0] != 1 {
		t.Fatalf("answered question key = %v, want [1]", rep.Answers[0].Correct)
	}

	// Starting a lab and handing it straight in must not be a way to read the
	// answers and score full marks on the next attempt.
	blank := sessionID + "-blank"
	// Already ended, because one running session per user is a unique index and
	// the fixture above holds that slot.
	if err := db.Exec(
		`INSERT INTO lab_sessions (id, user_id, lab_id, expires_at, status)
		 VALUES (?, ?, ?, now() + interval '1 hour', 'submitted')`, blank, userID, labID,
	).Error; err != nil {
		t.Fatalf("seed blank session: %v", err)
	}
	unanswered, err := r.Report(ctx, blank)
	if err != nil {
		t.Fatalf("report of unanswered session: %v", err)
	}
	for i, a := range unanswered.Answers {
		if len(a.Correct) != 0 {
			t.Fatalf("unanswered question %d leaked key %v", i, a.Correct)
		}
	}

	hist, err := r.History(ctx, userID)
	if err != nil || len(hist) != 2 || hist[0].Correct+hist[1].Correct != 2 {
		t.Fatalf("history = %+v (err %v), want two rows and 2 correct total", hist, err)
	}
}

// seedGradeFixture builds a throwaway user, course, lab and two tasks, and
// removes them again when the test ends. The cascades do most of that: deleting
// the user takes its scores and completions with it.
func seedGradeFixture(t *testing.T, db *gorm.DB) (userID, courseID, labID int64, sessionID string, taskIDs []int64) {
	t.Helper()
	suffix := time.Now().UnixNano()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	must(db.Raw(
		`INSERT INTO users (username, email, password_hash, status)
		 VALUES (?, ?, 'x', 'active') RETURNING id`,
		fmt.Sprintf("grade-test-%d", suffix), fmt.Sprintf("grade-%d@test.local", suffix),
	).Scan(&userID).Error)
	must(db.Raw(
		`INSERT INTO courses (slug, title, description, level, status)
		 VALUES (?, 'grade test', '', 'beginner', 'draft') RETURNING id`,
		fmt.Sprintf("grade-test-%d", suffix),
	).Scan(&courseID).Error)
	must(db.Raw(
		`INSERT INTO labs (course_id, slug, title, description_md, duration_minutes)
		 VALUES (?, ?, 'grade test lab', '', 30) RETURNING id`,
		courseID, fmt.Sprintf("grade-test-lab-%d", suffix),
	).Scan(&labID).Error)
	// One is a choice question with a key, so a report can be checked for
	// handing that key over; two is a plain script task.
	must(db.Raw(
		`INSERT INTO lab_tasks (lab_id, title, points, check_script, order_idx, kind, options)
		 VALUES (?, 'one', 10, '', 0, 'choice',
		         '[{"text":"a","correct":false},{"text":"b","correct":true}]'::jsonb),
		        (?, 'two', 15, 'true', 1, 'script', '[]'::jsonb) RETURNING id`,
		labID, labID,
	).Scan(&taskIDs).Error)

	// lab_answers references this, so grading has to have a session to hang off
	// the same way it does in the application.
	sessionID = fmt.Sprintf("grade-test-sess-%d", suffix)
	must(db.Exec(
		`INSERT INTO lab_sessions (id, user_id, lab_id, expires_at)
		 VALUES (?, ?, ?, now() + interval '1 hour')`,
		sessionID, userID, labID,
	).Error)

	t.Cleanup(func() {
		db.Exec(`DELETE FROM users WHERE id = ?`, userID)
		db.Exec(`DELETE FROM courses WHERE id = ?`, courseID)
	})
	return userID, courseID, labID, sessionID, taskIDs
}
