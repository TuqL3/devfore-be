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
)

// The gate that stops somebody spinning up a container on a course they never
// signed up for. Worth a test of its own because the query joins through the
// lab to the course — a wrong join here answers "enrolled" for every lab on the
// platform, and nothing about the screen would look different.
func TestIsEnrolledFollowsTheCourse(t *testing.T) {
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
	r := NewSessionRepo(db)
	suffix := time.Now().UnixNano()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	var userID, mineID, theirsID int64
	must(db.Raw(
		`INSERT INTO users (username, email, password_hash, status)
		 VALUES (?, ?, 'x', 'active') RETURNING id`,
		fmt.Sprintf("enrol-test-%d", suffix), fmt.Sprintf("enrol-%d@test.local", suffix),
	).Scan(&userID).Error)
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, userID) })

	// Two courses, one lab each. The student enrols in exactly one of them.
	for i, dst := range []*int64{&mineID, &theirsID} {
		must(db.Raw(
			`INSERT INTO courses (slug, title, description, level, status)
			 VALUES (?, 'enrol test', '', 'beginner', 'draft') RETURNING id`,
			fmt.Sprintf("enrol-test-%d-%d", suffix, i),
		).Scan(dst).Error)
		must(db.Exec(
			`INSERT INTO labs (course_id, slug, title, description_md, duration_minutes)
			 VALUES (?, ?, 'enrol test lab', '', 30)`,
			*dst, fmt.Sprintf("enrol-test-lab-%d-%d", suffix, i),
		).Error)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM courses WHERE id IN (?, ?)`, mineID, theirsID) })

	mineSlug := fmt.Sprintf("enrol-test-lab-%d-0", suffix)
	theirsSlug := fmt.Sprintf("enrol-test-lab-%d-1", suffix)

	// Before enrolling, neither lab is open to them.
	for _, slug := range []string{mineSlug, theirsSlug} {
		ok, err := r.IsEnrolled(ctx, userID, slug)
		if err != nil {
			t.Fatalf("IsEnrolled(%s): %v", slug, err)
		}
		if ok {
			t.Fatalf("chưa đăng ký khoá nào mà %s đã mở", slug)
		}
	}

	must(db.Exec(`INSERT INTO enrollments (user_id, course_id) VALUES (?, ?)`, userID, mineID).Error)

	ok, err := r.IsEnrolled(ctx, userID, mineSlug)
	if err != nil {
		t.Fatalf("IsEnrolled: %v", err)
	}
	if !ok {
		t.Fatal("đã đăng ký khoá mà lab của chính khoá đó vẫn bị chặn")
	}

	// The important half: enrolling in one course must not open the other.
	ok, err = r.IsEnrolled(ctx, userID, theirsSlug)
	if err != nil {
		t.Fatalf("IsEnrolled: %v", err)
	}
	if ok {
		t.Fatal("đăng ký một khoá lại mở được lab của khoá khác")
	}
}
