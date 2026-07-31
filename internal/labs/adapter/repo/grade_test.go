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
	userID, courseID, labID, taskIDs := seedGradeFixture(t, db)

	one := &domain.Task{ID: taskIDs[0], LabID: labID, CourseID: courseID, Points: 10}
	two := &domain.Task{ID: taskIDs[1], LabID: labID, CourseID: courseID, Points: 15}

	// A failed attempt counts as an attempt and nothing else.
	if g, err := r.Record(ctx, userID, "sess-1", one, false); err != nil {
		t.Fatalf("record fail: %v", err)
	} else if g.PointsAwarded != 0 || g.Passed {
		t.Fatalf("failed check awarded %+v", g)
	}

	g, err := r.Record(ctx, userID, "sess-1", one, true)
	if err != nil {
		t.Fatalf("record pass: %v", err)
	}
	if g.PointsAwarded != 10 || g.LabCompleted {
		t.Fatalf("first pass = %+v, want 10 points and lab not complete", g)
	}

	// The retry is the case the primary key exists for.
	if g, err = r.Record(ctx, userID, "sess-1", one, true); err != nil {
		t.Fatalf("record repeat: %v", err)
	} else if g.PointsAwarded != 0 {
		t.Fatalf("repeat pass awarded %d points, want 0", g.PointsAwarded)
	}

	if g, err = r.Record(ctx, userID, "sess-1", two, true); err != nil {
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

	ids, err := r.PassedTaskIDs(ctx, userID, labID)
	if err != nil || len(ids) != 2 {
		t.Fatalf("passed ids = %v (err %v), want 2", ids, err)
	}
}

// seedGradeFixture builds a throwaway user, course, lab and two tasks, and
// removes them again when the test ends. The cascades do most of that: deleting
// the user takes its scores and completions with it.
func seedGradeFixture(t *testing.T, db *gorm.DB) (userID, courseID, labID int64, taskIDs []int64) {
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
	must(db.Raw(
		`INSERT INTO lab_tasks (lab_id, title, points, check_script, order_idx)
		 VALUES (?, 'one', 10, 'true', 0), (?, 'two', 15, 'true', 1) RETURNING id`,
		labID, labID,
	).Scan(&taskIDs).Error)

	t.Cleanup(func() {
		db.Exec(`DELETE FROM users WHERE id = ?`, userID)
		db.Exec(`DELETE FROM courses WHERE id = ?`, courseID)
	})
	return userID, courseID, labID, taskIDs
}
