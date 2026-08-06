// Command checksim proves that every seeded sim task can actually be passed,
// and cannot be passed by a pipeline that misses the point.
//
//	make check-sim
//
// A goal that nobody can satisfy and a goal that everybody satisfies look
// identical in the database — both are a bit of JSON that parses. The only way
// to tell them apart is to run pipelines against them, so each task carries two:
//
//	scripts/sim-pipelines/<lab-slug>/<idx>-fail.yml   MUST NOT pass
//	scripts/sim-pipelines/<lab-slug>/<idx>-pass.yml   MUST pass
//
// The fail file is the half that matters. A goal of `{"run_status":"success"}`
// on a lab about parallelism is passed by the serial pipeline the student
// started with, and nothing else in the system would ever say so.
//
// This is the grader's own code — sim.Parse, sim.Run, sim.Eval — not a copy of
// it, so what passes here is what passes a student.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/gorm"

	"github.com/devforge/be/internal/config"
	"github.com/devforge/be/internal/db"
	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase/sim"
)

// pipelineDir holds one directory per sim lab, named by its slug.
const pipelineDir = "scripts/sim-pipelines"

// runsPerAttempt is how many times a pipeline is run before its last result is
// graded. Two, because a cache only warms for the next run: a task asking for a
// cache hit is unreachable in one, and a student meeting it presses Run twice.
//
// Grading the warm run is the generous direction — every goal in this vocabulary
// is easier to meet warm than cold — so a "must fail" pipeline that still fails
// here fails under the best conditions it could have had.
const runsPerAttempt = 2

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lỗi:", err)
		os.Exit(2)
	}
}

type simLab struct {
	Slug        string
	SimScenario []byte
}

type simTask struct {
	OrderIdx int
	Title    string
	SimGoal  []byte
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	gdb, err := db.Open(cfg.DSN(), true)
	if err != nil {
		return err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	ctx := context.Background()
	labs := []simLab{}
	if err := gdb.WithContext(ctx).Raw(
		`SELECT slug, sim_scenario FROM labs
		  WHERE sim_scenario IS NOT NULL ORDER BY course_id, order_idx, id`,
	).Scan(&labs).Error; err != nil {
		return err
	}
	if len(labs) == 0 {
		// Not an error: a database with no sim lab in it has nothing wrong with
		// it. Saying so beats a green tick that checked nothing.
		fmt.Println("không có lab mô phỏng nào trong database")
		return nil
	}

	failures := 0
	checked := 0
	for _, lab := range labs {
		fmt.Printf("── %s\n", lab.Slug)
		n, bad, err := checkLab(ctx, gdb, lab)
		if err != nil {
			return err
		}
		checked += n
		failures += bad
	}

	// Sân chơi không còn ở đây: kịch bản của nó nằm trong code frontend, nên
	// tsc và bản build là thứ bắt lỗi, không phải lệnh này.
	fmt.Printf("\nđã kiểm %d nhiệm vụ mô phỏng, %d lỗi\n", checked, failures)
	if failures > 0 {
		os.Exit(1)
	}
	return nil
}

func checkLab(ctx context.Context, gdb *gorm.DB, lab simLab) (checked, failures int, err error) {
	var sc domain.Scenario
	if err := json.Unmarshal(lab.SimScenario, &sc); err != nil {
		fmt.Printf("  ✗ kịch bản không đọc được: %v\n", err)
		return 0, 1, nil
	}
	if len(sc.Catalog) == 0 {
		fmt.Println("  ✗ kịch bản không có step nào — không viết pipeline gì được")
		return 0, 1, nil
	}
	if sc.RunnerCount < 1 {
		fmt.Printf("  ✗ runner_count = %d — không job nào chạy được\n", sc.RunnerCount)
		return 0, 1, nil
	}

	tasks := []simTask{}
	if err := gdb.WithContext(ctx).Raw(
		`SELECT t.order_idx, t.title, t.sim_goal
		   FROM lab_tasks t JOIN labs l ON l.id = t.lab_id
		  WHERE l.slug = ? AND t.kind = 'sim'
		  ORDER BY t.order_idx`, lab.Slug,
	).Scan(&tasks).Error; err != nil {
		return 0, 0, err
	}
	if len(tasks) == 0 {
		fmt.Println("  ✗ lab mô phỏng nhưng không có nhiệm vụ kind='sim' nào")
		return 0, 1, nil
	}

	for _, task := range tasks {
		checked++
		if bad := checkTask(&sc, lab.Slug, task); bad {
			failures++
		}
	}
	return checked, failures, nil
}

func checkTask(sc *domain.Scenario, labSlug string, task simTask) (failed bool) {
	label := fmt.Sprintf("[%d] %s", task.OrderIdx, task.Title)

	var goal domain.Goal
	if err := json.Unmarshal(task.SimGoal, &goal); err != nil {
		fmt.Printf("  ✗ %s — sim_goal không đọc được: %v\n", label, err)
		return true
	}
	if len(goal.All) == 0 {
		fmt.Printf("  ✗ %s — chưa có điều kiện chấm, không ai đạt được\n", label)
		return true
	}

	// Reported together rather than returning on the first problem: an author
	// fixing content wants both halves of the verdict in one run.
	bad := false
	for _, want := range []bool{false, true} {
		name := "fail"
		if want {
			name = "pass"
		}
		path := filepath.Join(pipelineDir, labSlug, fmt.Sprintf("%d-%s.yml", task.OrderIdx, name))
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("  ✗ %s — thiếu %s\n", label, path)
			bad = true
			continue
		}

		got, res, err := grade(sc, string(src), &goal)
		switch {
		case err != nil:
			// A pass file that will not parse is a broken example. A fail file
			// that will not parse is worse: it "fails" for a reason that has
			// nothing to do with the goal, so it proves nothing about it.
			fmt.Printf("  ✗ %s — %s.yml không chạy được: %v\n", label, name, err)
			bad = true
		case got != want && want:
			fmt.Printf("  ✗ %s — pass.yml TRƯỢT (pipeline %s, %ds)\n",
				label, res.Status, res.TotalSeconds)
			bad = true
		case got != want:
			fmt.Printf("  ✗ %s — fail.yml lại ĐẬU (điều kiện chấm quá lỏng)\n", label)
			bad = true
		}
	}
	if !bad {
		fmt.Printf("  ✓ %s\n", label)
	}
	return bad
}

// grade runs one pipeline the way a session does — several presses of Run in a
// row, each inheriting the caches the last one left warm — and evaluates the
// goal against the final result, which is the one grading reads.
func grade(
	sc *domain.Scenario, src string, goal *domain.Goal,
) (bool, *domain.RunResult, error) {
	p, err := sim.Parse(src, sc)
	if err != nil {
		return false, nil, err
	}
	var res *domain.RunResult
	var warm []string
	for i := 1; i <= runsPerAttempt; i++ {
		// A fixed seed: this command has to answer the same way on every machine
		// and every run, or a red build here means nothing.
		res = sim.Run(sc, p, "check-sim", i, warm)
		warm = res.WarmCaches
	}
	passed, err := sim.Eval(goal, res)
	return passed, res, err
}
