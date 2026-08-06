package repo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/devforge/be/internal/labs/domain"
)

// The join in SpecBySlug had to go from inner to left so a sim lab — which has
// no lab_images row at all — stops reading as a lab that does not exist. That
// change runs under every press of Start on the platform, and the way it fails
// is silent in both directions: too strict and no sim lab starts, too loose and
// a lab with a deactivated image is handed to runtime.Create with an empty
// image name instead of being refused. All three cases are checked here.
func TestSpecBySlugFindsBothKindsOfLab(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := NewSessionRepo(db)
	suffix := time.Now().UnixNano()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	var courseID, imageID int64
	must(db.Raw(
		`INSERT INTO courses (slug, title, description, level, status)
		 VALUES (?, 'sim test', '', 'beginner', 'published') RETURNING id`,
		fmt.Sprintf("sim-test-%d", suffix),
	).Scan(&courseID).Error)
	t.Cleanup(func() { db.Exec(`DELETE FROM courses WHERE id = ?`, courseID) })

	must(db.Raw(
		`INSERT INTO lab_images (name, tag, active) VALUES (?, 'v1', true) RETURNING id`,
		fmt.Sprintf("devforge/sim-test-%d", suffix),
	).Scan(&imageID).Error)
	t.Cleanup(func() { db.Exec(`DELETE FROM lab_images WHERE id = ?`, imageID) })

	newLab := func(kind string, imageID *int64, scenario *string) string {
		slug := fmt.Sprintf("sim-test-lab-%s-%d", kind, suffix)
		must(db.Exec(
			`INSERT INTO labs (course_id, slug, title, description_md, duration_minutes,
			                   lab_image_id, sim_scenario)
			 VALUES (?, ?, 'sim test lab', '', 30, ?, ?::jsonb)`,
			courseID, slug, imageID, scenario,
		).Error)
		return slug
	}

	scenario := `{"version":1,"runner_count":2,"catalog":{"checkout":{"seconds":5}}}`
	containerSlug := newLab("container", &imageID, nil)
	simSlug := newLab("sim", nil, &scenario)
	emptySlug := newLab("empty", nil, nil)

	// The lab that already worked has to keep working, image and all.
	spec, err := r.SpecBySlug(ctx, containerSlug)
	if err != nil {
		t.Fatalf("SpecBySlug(container): %v", err)
	}
	if spec.Image == "" {
		t.Fatal("lab container mất image sau khi đổi sang LEFT JOIN")
	}
	if len(spec.SimScenario) != 0 {
		t.Fatalf("lab container lại có kịch bản: %s", spec.SimScenario)
	}

	// The lab the change exists for.
	spec, err = r.SpecBySlug(ctx, simSlug)
	if err != nil {
		t.Fatalf("SpecBySlug(sim): %v", err)
	}
	if spec.Image != "" {
		t.Fatalf("lab sim lại có image: %q", spec.Image)
	}
	if len(spec.SimScenario) == 0 {
		t.Fatal("lab sim không đọc được sim_scenario")
	}

	// Neither runtime is still nothing to start, the same answer the inner join
	// used to give.
	if _, err := r.SpecBySlug(ctx, emptySlug); !errors.Is(err, domain.ErrLabNotFound) {
		t.Fatalf("lab không có image lẫn kịch bản: mong ErrLabNotFound, nhận %v", err)
	}

	// An image that was switched off is not a runtime either. This is the case the
	// left join would quietly let through if `i.active` sat in the WHERE clause
	// with the guard missing.
	must(db.Exec(`UPDATE lab_images SET active = false WHERE id = ?`, imageID).Error)
	if _, err := r.SpecBySlug(ctx, containerSlug); !errors.Is(err, domain.ErrLabNotFound) {
		t.Fatalf("image đã tắt: mong ErrLabNotFound, nhận %v", err)
	}
}

// A run has to come back out of jsonb the way it went in, because grading reads
// it from there rather than from the engine that produced it — and the next run
// inherits its warm caches from the same row.
func TestSaveRunRoundTrips(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := NewSimRepo(db)
	suffix := time.Now().UnixNano()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	var userID, courseID, labID int64
	must(db.Raw(
		`INSERT INTO users (username, email, password_hash, status)
		 VALUES (?, ?, 'x', 'active') RETURNING id`,
		fmt.Sprintf("sim-run-%d", suffix), fmt.Sprintf("sim-run-%d@test.local", suffix),
	).Scan(&userID).Error)
	t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id = ?`, userID) })

	must(db.Raw(
		`INSERT INTO courses (slug, title, description, level, status)
		 VALUES (?, 'sim run test', '', 'beginner', 'published') RETURNING id`,
		fmt.Sprintf("sim-run-course-%d", suffix),
	).Scan(&courseID).Error)
	t.Cleanup(func() { db.Exec(`DELETE FROM courses WHERE id = ?`, courseID) })

	must(db.Raw(
		`INSERT INTO labs (course_id, slug, title, description_md, duration_minutes, sim_scenario)
		 VALUES (?, ?, 'sim run lab', '', 30,
		         '{"version":1,"runner_count":2,"catalog":{"checkout":{"seconds":5}}}'::jsonb)
		 RETURNING id`,
		courseID, fmt.Sprintf("sim-run-lab-%d", suffix),
	).Scan(&labID).Error)

	sessionID := fmt.Sprintf("sim-sess-%d", suffix)
	must(db.Exec(
		`INSERT INTO lab_sessions (id, user_id, lab_id, expires_at)
		 VALUES (?, ?, ?, now() + interval '1 hour')`,
		sessionID, userID, labID,
	).Error)
	t.Cleanup(func() { db.Exec(`DELETE FROM lab_sessions WHERE id = ?`, sessionID) })

	// The scenario the lab was seeded with is readable back as a scenario.
	sc, err := r.ScenarioByLab(ctx, labID)
	if err != nil {
		t.Fatalf("ScenarioByLab: %v", err)
	}
	if sc.RunnerCount != 2 || len(sc.Catalog) != 1 {
		t.Fatalf("kịch bản đọc ra sai: %+v", sc)
	}

	// Nothing run yet is its own answer, not an empty result.
	if _, err := r.LatestRun(ctx, sessionID); !errors.Is(err, domain.ErrNoSimRun) {
		t.Fatalf("chưa chạy lượt nào: mong ErrNoSimRun, nhận %v", err)
	}

	for i := 1; i <= 2; i++ {
		must(r.SaveRun(ctx, sessionID, &domain.SimRun{
			RunIndex: i,
			Pipeline: fmt.Sprintf("jobs:\n  build:\n    steps: [checkout] # %d\n", i),
			Result: &domain.RunResult{
				RunIndex:     i,
				Status:       domain.RunSuccess,
				TotalSeconds: 5 * i,
				WarmCaches:   []string{fmt.Sprintf("node_modules_%d", i)},
				Jobs: []domain.RunJob{{
					Name: "build", Runner: 0, Start: 0, End: 5,
					Status: domain.JobSuccess,
					Steps: []domain.RunStep{{
						Uses: "checkout", Start: 0, End: 5, Status: domain.JobSuccess,
					}},
				}},
			},
		}))
	}

	// Latest is the highest run number, and its warm caches survived the round
	// trip — that list is what the next run starts from.
	latest, err := r.LatestRun(ctx, sessionID)
	if err != nil {
		t.Fatalf("LatestRun: %v", err)
	}
	if latest.RunIndex != 2 {
		t.Fatalf("LatestRun trả lượt %d, mong 2", latest.RunIndex)
	}
	if len(latest.Result.WarmCaches) != 1 || latest.Result.WarmCaches[0] != "node_modules_2" {
		t.Fatalf("warm_caches không sống sót qua jsonb: %v", latest.Result.WarmCaches)
	}
	if len(latest.Result.Jobs) != 1 || len(latest.Result.Jobs[0].Steps) != 1 {
		t.Fatalf("job/step không sống sót qua jsonb: %+v", latest.Result.Jobs)
	}

	// The same run number twice is two presses of Run racing. Reported, not
	// silently renumbered.
	if err := r.SaveRun(ctx, sessionID, &domain.SimRun{
		RunIndex: 2, Pipeline: "jobs:\n  build:\n    steps: [checkout]\n",
		Result: &domain.RunResult{RunIndex: 2, Status: domain.RunSuccess},
	}); !errors.Is(err, domain.ErrRunRaced) {
		t.Fatalf("ghi trùng run_index: mong ErrRunRaced, nhận %v", err)
	}

	runs, err := r.Runs(ctx, sessionID)
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) != 2 || runs[0].RunIndex != 1 || runs[1].RunIndex != 2 {
		t.Fatalf("lịch sử sai thứ tự hoặc thiếu: %+v", runs)
	}
}

func testDB(t *testing.T) *gorm.DB {
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
