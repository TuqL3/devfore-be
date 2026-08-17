package repo

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// Start skips the enrolment check for any lab that carries an incident
// (internal/labs/usecase/labs.go). That is only safe while a drill lab has no
// course to enrol in. Both ways of breaking the pairing are silent from every
// screen — the lab keeps working, it just stops asking — so the guard belongs in
// the database and this is the test that says it is still there.
func TestIncidentCannotBeAttachedToALabThatHasACourse(t *testing.T) {
	db := testDB(t)

	courseID := seedCourse(t, db)
	labID := seedLabInCourse(t, db, courseID)

	err := db.Exec(
		`INSERT INTO lab_incidents (lab_id, title, break_script, active)
		 VALUES (?, 'should not land', 'true', true)`, labID,
	).Error
	if err == nil {
		t.Fatal("gắn kịch bản vào lab thuộc khoá học: muốn bị từ chối, nhưng đã chèn được")
	}
	if !strings.Contains(err.Error(), "enrolment gate") {
		t.Fatalf("từ chối vì lý do khác: %v", err)
	}
}

// The mirror image: the lab already carries an incident and somebody moves it
// into a course. lab_incidents is never written, so the trigger on that table
// cannot see it happen.
func TestDrillLabCannotBeMovedIntoACourse(t *testing.T) {
	db := testDB(t)

	labID := seedLab(t, db) // course-less, the shape a drill lab really has
	seedIncident(t, db, labID, "disk full", true)
	courseID := seedCourse(t, db)

	err := db.Exec(`UPDATE labs SET course_id = ? WHERE id = ?`, courseID, labID).Error
	if err == nil {
		t.Fatal("chuyển lab drill vào khoá học: muốn bị từ chối, nhưng đã cập nhật được")
	}
	if !strings.Contains(err.Error(), "enrolment gate") {
		t.Fatalf("từ chối vì lý do khác: %v", err)
	}
}

// An ordinary lab still belongs to a course, and an ordinary course still
// accepts labs. A guard that also blocked the normal path would be worse than
// the hole it closes.
func TestOrdinaryLabInACourseIsUntouched(t *testing.T) {
	db := testDB(t)

	courseID := seedCourse(t, db)
	labID := seedLabInCourse(t, db, courseID)

	if err := db.Exec(`UPDATE labs SET course_id = ? WHERE id = ?`, courseID, labID).Error; err != nil {
		t.Fatalf("lab thường trong khoá học bị chặn oan: %v", err)
	}
}

func seedCourse(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var id int64
	if err := db.Raw(
		`INSERT INTO courses (slug, title, description, level, status)
		 VALUES (?, 'drill guard test', '', 'beginner', 'draft') RETURNING id`,
		fmt.Sprintf("drill-guard-%d", time.Now().UnixNano()),
	).Scan(&id).Error; err != nil {
		t.Fatalf("seed course: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM courses WHERE id = ?`, id) })
	return id
}

func seedLabInCourse(t *testing.T, db *gorm.DB, courseID int64) int64 {
	t.Helper()
	var id int64
	if err := db.Raw(
		`INSERT INTO labs (course_id, slug, title, description_md, duration_minutes)
		 VALUES (?, ?, 'drill guard lab', '', 30) RETURNING id`,
		courseID, fmt.Sprintf("drill-guard-lab-%d", time.Now().UnixNano()),
	).Scan(&id).Error; err != nil {
		t.Fatalf("seed lab: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM labs WHERE id = ?`, id) })
	return id
}
