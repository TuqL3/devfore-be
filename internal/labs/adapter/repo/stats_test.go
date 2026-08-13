package repo

import (
	"context"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/devforge/be/internal/labs/domain"
)

// The per-lab table joins sessions to their answers, which multiplies each
// session by however many questions it answered. Getting that wrong reports a
// ten-question lab as ten times as popular as a one-question one, and the number
// looks plausible enough that nobody would question it.
//
// Asserted against the seeded lab's own row rather than the platform totals: the
// database this runs on is shared, so every global count is whatever else is in
// it at the time.
func TestStatsCountsSessionsOncePerLab(t *testing.T) {
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

	one := &domain.Task{ID: taskIDs[0], LabID: labID, CourseID: &courseID, Points: 10}
	two := &domain.Task{ID: taskIDs[1], LabID: labID, CourseID: &courseID, Points: 15}

	// Task one takes two presses to pass, task two one. Two answer rows on a
	// single session, and one of them retried.
	if _, err := r.Record(ctx, userID, sessionID, one, []int{1}, false); err != nil {
		t.Fatalf("record fail: %v", err)
	}
	if _, err := r.Record(ctx, userID, sessionID, one, []int{0}, true); err != nil {
		t.Fatalf("record pass: %v", err)
	}
	if _, err := r.Record(ctx, userID, sessionID, two, nil, true); err != nil {
		t.Fatalf("record second task: %v", err)
	}

	s, err := r.Stats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}

	var row *domain.LabStat
	for i := range s.Labs {
		if s.Labs[i].LabID == labID {
			row = &s.Labs[i]
			break
		}
	}
	if row == nil {
		t.Fatalf("lab %d missing from stats", labID)
	}
	// One session, however many answers hang off it.
	if row.Sessions != 1 {
		t.Fatalf("sessions = %d, want 1 — the join to lab_answers is being counted", row.Sessions)
	}
	if row.Submitted != 0 {
		t.Fatalf("submitted = %d, want 0 — the session was never handed in", row.Submitted)
	}
	if row.Answered != 2 {
		t.Fatalf("answered = %d, want 2", row.Answered)
	}
	if row.Retried != 1 {
		t.Fatalf("retried = %d, want 1 — only task one took a second press", row.Retried)
	}
	if row.CourseTitle == "" {
		t.Fatalf("course title empty, want the lab's own course")
	}

	// A lab nobody has opened still belongs in the table, as the zero it is.
	var idleID int64
	if err := db.Raw(
		`INSERT INTO labs (course_id, slug, title, description_md, duration_minutes)
		 VALUES (?, ?, 'idle lab', '', 30) RETURNING id`,
		courseID, "stats-test-idle-lab",
	).Scan(&idleID).Error; err != nil {
		t.Fatalf("seed idle lab: %v", err)
	}
	if s, err = r.Stats(ctx); err != nil {
		t.Fatalf("stats after idle lab: %v", err)
	}
	found := false
	for _, l := range s.Labs {
		if l.LabID == idleID {
			found = true
			if l.Sessions != 0 || l.Answered != 0 {
				t.Fatalf("idle lab = %+v, want zeroes", l)
			}
		}
	}
	if !found {
		t.Fatal("lab with no sessions dropped from stats — the join has to be a LEFT one")
	}
}

// The kill list drives a button that destroys someone's work in progress, so
// what it does and does not list is the whole safety of it: a session that has
// already ended appearing here is an admin killing a container that is not
// theirs to kill, and a live one missing is a container nobody can reach.
func TestRunningSessionsListsOnlyLiveOnes(t *testing.T) {
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
	_, _, _, sessionID, _ := seedGradeFixture(t, db)

	live, err := r.RunningSessions(ctx)
	if err != nil {
		t.Fatalf("running sessions: %v", err)
	}
	row := findSession(live, sessionID)
	if row == nil {
		t.Fatalf("seeded running session %s missing from the list", sessionID)
	}
	if row.Username == "" || row.LabTitle == "" {
		t.Fatalf("row = %+v, want the user and lab resolved, not just ids", *row)
	}
	// The fixture never attaches a container, which is the shape an orphaned
	// session takes and is worth telling apart on screen.
	if row.HasContainer {
		t.Fatalf("row = %+v, want has_container false for a session with no container", *row)
	}

	if err := db.Exec(
		`UPDATE lab_sessions SET status = 'ended', ended_at = now() WHERE id = ?`, sessionID,
	).Error; err != nil {
		t.Fatalf("end session: %v", err)
	}
	if live, err = r.RunningSessions(ctx); err != nil {
		t.Fatalf("running sessions after end: %v", err)
	}
	if findSession(live, sessionID) != nil {
		t.Fatal("ended session still listed as running")
	}
}

func findSession(rows []domain.RunningSession, id string) *domain.RunningSession {
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i]
		}
	}
	return nil
}
