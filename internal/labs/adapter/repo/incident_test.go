package repo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/devforge/be/internal/labs/domain"
)

// The draw is the whole of what makes an incident lab replayable, and two ways
// it can be wrong are both invisible from the screen: handing out a retired
// scenario, or never handing out one of the live ones. Both would look like a
// working lab to anyone who did not count.
func TestPickIncidentDrawsEveryLiveScenarioAndNoRetiredOne(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := NewSessionRepo(db)

	labID := seedLab(t, db)

	// A lab with no scenarios is an ordinary lab. That answer is what Start
	// branches on, so it is part of the contract rather than an edge case.
	if _, err := r.PickIncident(ctx, labID); !errors.Is(err, domain.ErrNoIncident) {
		t.Fatalf("lab chưa có kịch bản: muốn ErrNoIncident, nhận %v", err)
	}

	first := seedIncident(t, db, labID, "disk full", true)
	second := seedIncident(t, db, labID, "port taken", true)
	retired := seedIncident(t, db, labID, "old one", false)

	// Sixty draws over two live rows: the odds of missing one by chance are
	// 2^-59, which is far enough from "flaky" to assert on.
	seen := map[int64]int{}
	for range 60 {
		inc, err := r.PickIncident(ctx, labID)
		if err != nil {
			t.Fatalf("PickIncident: %v", err)
		}
		if inc.ID == retired {
			t.Fatal("bốc trúng kịch bản đã nghỉ hưu (active = false)")
		}
		seen[inc.ID]++
	}
	if seen[first] == 0 || seen[second] == 0 {
		t.Fatalf("có kịch bản không bao giờ được bốc: %v", seen)
	}

	// A report has to name the fault after the author retires it, so reading one
	// back by id ignores the flag that stops it being handed out.
	inc, err := r.IncidentByID(ctx, retired)
	if err != nil {
		t.Fatalf("IncidentByID(kịch bản đã nghỉ hưu): %v", err)
	}
	if inc.Title != "old one" || inc.BreakScript == "" {
		t.Fatalf("đọc lại kịch bản ra sai nội dung: %+v", inc)
	}
}

// A session carries the fault it drew, and reads it back. Worth its own test
// because the column is nullable and every ordinary lab leaves it null: a scan
// that cannot cope with that would break sessions that have nothing to do with
// incidents, which is every session on the platform today.
func TestSessionRemembersWhichIncidentItDrew(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := NewSessionRepo(db)

	labID := seedLab(t, db)
	incidentID := seedIncident(t, db, labID, "disk full", true)

	suffix := time.Now().UnixNano()
	var userID int64
	if err := db.Raw(
		`INSERT INTO users (username, email, password_hash, status)
		 VALUES (?, ?, 'x', 'active') RETURNING id`,
		fmt.Sprintf("incident-test-%d", suffix), fmt.Sprintf("incident-%d@test.local", suffix),
	).Scan(&userID).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, userID) })

	s := &domain.Session{
		ID:        fmt.Sprintf("incident-sess-%d", suffix),
		UserID:    userID,
		LabID:     labID,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := r.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Fresh row: no fault drawn yet, which is also what every container lab
	// looks like for the whole of its life.
	got, err := r.ByID(ctx, s.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.IncidentID != nil {
		t.Fatalf("phiên mới mà đã có incident_id: %v", *got.IncidentID)
	}

	if err := r.SetIncident(ctx, s.ID, incidentID); err != nil {
		t.Fatalf("SetIncident: %v", err)
	}
	if err := r.SaveCommandLog(ctx, s.ID, "#1786400000\ndf -h\n"); err != nil {
		t.Fatalf("SaveCommandLog: %v", err)
	}

	got, err = r.ByID(ctx, s.ID)
	if err != nil {
		t.Fatalf("ByID sau khi ghi: %v", err)
	}
	if got.IncidentID == nil || *got.IncidentID != incidentID {
		t.Fatalf("đọc lại incident_id sai: %v", got.IncidentID)
	}

	var log string
	if err := db.Raw(`SELECT command_log FROM lab_sessions WHERE id = ?`, s.ID).Scan(&log).Error; err != nil {
		t.Fatalf("đọc command_log: %v", err)
	}
	if log != "#1786400000\ndf -h\n" {
		t.Fatalf("command_log lưu sai: %q", log)
	}
}

// seedLab makes a lab shaped like a real drill lab: no course. That is not a
// shortcut to skip seeding one — since migration 000028 it is what a drill lab
// is, and migration 000034 refuses to attach an incident to any lab that does
// have a course. A helper that seeded a course here would be testing a shape
// the database no longer accepts.
func seedLab(t *testing.T, db *gorm.DB) (labID int64) {
	t.Helper()
	suffix := time.Now().UnixNano()
	if err := db.Raw(
		`INSERT INTO labs (course_id, slug, title, description_md, duration_minutes)
		 VALUES (NULL, ?, 'incident test lab', '', 30) RETURNING id`,
		fmt.Sprintf("incident-test-lab-%d", suffix),
	).Scan(&labID).Error; err != nil {
		t.Fatalf("seed lab: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM labs WHERE id = ?`, labID) })
	return labID
}

func seedIncident(t *testing.T, db *gorm.DB, labID int64, title string, active bool) int64 {
	t.Helper()
	var id int64
	if err := db.Raw(
		`INSERT INTO lab_incidents (lab_id, title, break_script, active)
		 VALUES (?, ?, 'true', ?) RETURNING id`, labID, title, active,
	).Scan(&id).Error; err != nil {
		t.Fatalf("seed incident: %v", err)
	}
	return id
}
