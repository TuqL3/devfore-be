// Package repo_test rather than repo: this file calls into usecase, and usecase
// imports repo. An external test package is the one place Go allows that, and
// holding the two definitions of "recovered" against each other is worth the
// extra package.
package repo_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/devforge/be/internal/labs/adapter/repo"
	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase"
)

// "The service came back" is written down twice: once in Go for the private
// report (usecase.RecoveredAt) and once in SQL for the leaderboard and the
// public page (repo.recoveredSQL). They say the same thing today. Nothing but
// this test stops them saying different things tomorrow — and the shape of that
// bug is a board listing somebody whose own report screen says they never fixed
// it, which nobody would think to check.
//
// The cases are the ones where the two could plausibly part company: a task
// never answered is not the same as a task answered wrong, and an empty lab is
// not the same as a finished one.
func TestRecoveredMeansTheSameThingInGoAndInSQL(t *testing.T) {
	db := crossTestDB(t)
	ctx := context.Background()
	r := repo.NewSessionRepo(db)

	labID, taskIDs := seedDrillLab(t, db, 3)
	incidentID := seedCrossIncident(t, db, labID)
	userID := seedCrossUser(t, db)
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, userID) })

	started := time.Now().Add(-10 * time.Minute).UTC()
	pass := func(at time.Time) answer { return answer{passed: true, at: &at} }

	cases := []struct {
		name    string
		answers []answer
	}{
		{"mọi nhiệm vụ đều đậu", []answer{
			pass(started.Add(60 * time.Second)),
			pass(started.Add(200 * time.Second)),
			pass(started.Add(372 * time.Second)),
		}},
		{"một nhiệm vụ trượt", []answer{
			pass(started.Add(60 * time.Second)),
			{passed: false, at: ptr(started.Add(90 * time.Second))},
			pass(started.Add(372 * time.Second)),
		}},
		{"một nhiệm vụ chưa trả lời", []answer{
			pass(started.Add(60 * time.Second)),
			pass(started.Add(200 * time.Second)),
		}},
		{"chưa trả lời gì cả", nil},
	}

	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sessionID := fmt.Sprintf("cross-%d-%d", time.Now().UnixNano(), i)
			token := "cross-token-" + sessionID
			seedCrossSession(t, db, sessionID, token, userID, labID, incidentID, started)
			writeAnswers(t, db, sessionID, taskIDs, c.answers)

			// SQL's verdict, read the way the public page reads it.
			shared, err := r.SharedByToken(ctx, token)
			if err != nil {
				t.Fatalf("SharedByToken: %v", err)
			}

			// Go's verdict, on the same rows. A report carries one answer per task
			// of the lab, so a task never answered is a row with no verdict — the
			// case that separates "wrong" from "not attempted".
			rep := &domain.Report{StartedAt: started}
			for j := range taskIDs {
				a := domain.ReportAnswer{TaskID: taskIDs[j]}
				if j < len(c.answers) {
					p := c.answers[j].passed
					a.Passed = &p
					a.AnsweredAt = c.answers[j].at
				}
				rep.Answers = append(rep.Answers, a)
			}
			goSays := usecase.RecoveredAt(rep) != nil

			if goSays != shared.Recovered {
				t.Fatalf("hai định nghĩa lệch nhau: Go nói recovered=%v, SQL nói %v",
					goSays, shared.Recovered)
			}

			// And when they agree it recovered, they have to agree on when — the
			// board ranks by that number.
			if goSays {
				want := int(usecase.RecoveredAt(rep).Sub(started).Seconds())
				if shared.DowntimeSeconds != want {
					t.Fatalf("thời gian chết lệch: SQL %ds, Go %ds",
						shared.DowntimeSeconds, want)
				}
			}
		})
	}

	// A lab with no tasks at all: SQL divides nothing by nothing, Go iterates an
	// empty list. Both have to answer "not recovered" — a drill nobody can fail is
	// not a drill, and the first draft of either could easily have said yes.
	emptyLabID, _ := seedDrillLab(t, db, 0)
	emptyIncident := seedCrossIncident(t, db, emptyLabID)
	sessionID := fmt.Sprintf("cross-empty-%d", time.Now().UnixNano())
	token := "cross-token-" + sessionID
	seedCrossSession(t, db, sessionID, token, userID, emptyLabID, emptyIncident, started)

	shared, err := r.SharedByToken(ctx, token)
	if err != nil {
		t.Fatalf("SharedByToken(lab rỗng): %v", err)
	}
	if shared.Recovered {
		t.Error("lab không có nhiệm vụ nào mà SQL vẫn nói đã cứu được")
	}
	if usecase.RecoveredAt(&domain.Report{StartedAt: started}) != nil {
		t.Error("lab không có nhiệm vụ nào mà Go vẫn nói đã cứu được")
	}
}

type answer struct {
	passed bool
	at     *time.Time
}

func ptr(t time.Time) *time.Time { return &t }

func crossTestDB(t *testing.T) *gorm.DB {
	t.Helper()
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
	return db
}

// A drill lab has no course — that is what it is, since migration 000028, and
// migration 000034 refuses to attach an incident to a lab that has one.
func seedDrillLab(t *testing.T, db *gorm.DB, tasks int) (labID int64, taskIDs []int64) {
	t.Helper()
	suffix := time.Now().UnixNano()
	if err := db.Raw(
		`INSERT INTO labs (course_id, slug, title, description_md, duration_minutes)
		 VALUES (NULL, ?, 'cross test lab', '', 30) RETURNING id`,
		fmt.Sprintf("cross-test-lab-%d", suffix),
	).Scan(&labID).Error; err != nil {
		t.Fatalf("seed lab: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM labs WHERE id = ?`, labID) })
	for i := range tasks {
		var id int64
		if err := db.Raw(
			`INSERT INTO lab_tasks (lab_id, order_idx, kind, title, check_script, points)
			 VALUES (?, ?, 'script', ?, 'true', 1) RETURNING id`,
			labID, i+1, fmt.Sprintf("task %d", i+1),
		).Scan(&id).Error; err != nil {
			t.Fatalf("seed task: %v", err)
		}
		taskIDs = append(taskIDs, id)
	}
	return labID, taskIDs
}

func seedCrossIncident(t *testing.T, db *gorm.DB, labID int64) int64 {
	t.Helper()
	var id int64
	if err := db.Raw(
		`INSERT INTO lab_incidents (lab_id, title, break_script, rps, active)
		 VALUES (?, 'cross fault', 'true', 20, true) RETURNING id`, labID,
	).Scan(&id).Error; err != nil {
		t.Fatalf("seed incident: %v", err)
	}
	return id
}

func seedCrossUser(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var id int64
	name := fmt.Sprintf("cross_%d", time.Now().UnixNano())
	if err := db.Raw(
		`INSERT INTO users (username, email, password_hash, status)
		 VALUES (?, ?, 'x', 'active') RETURNING id`, name, name+"@example.test",
	).Scan(&id).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedCrossSession(
	t *testing.T, db *gorm.DB, id, token string, userID, labID, incidentID int64, started time.Time,
) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO lab_sessions
		   (id, user_id, lab_id, status, started_at, expires_at, incident_id, share_token)
		 VALUES (?, ?, ?, 'submitted', ?, ?, ?, ?)`,
		id, userID, labID, started, started.Add(time.Hour), incidentID, token,
	).Error; err != nil {
		t.Fatalf("seed session: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM lab_sessions WHERE id = ?`, id) })
}

func writeAnswers(t *testing.T, db *gorm.DB, sessionID string, taskIDs []int64, as []answer) {
	t.Helper()
	for i, a := range as {
		if i >= len(taskIDs) {
			break
		}
		if err := db.Exec(
			`INSERT INTO lab_answers (session_id, task_id, passed, answered_at)
			 VALUES (?, ?, ?, ?)`, sessionID, taskIDs[i], a.passed, a.at,
		).Error; err != nil {
			t.Fatalf("seed answer: %v", err)
		}
	}
}
