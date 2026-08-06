package sim

import (
	"errors"
	"testing"

	"github.com/devforge/be/internal/labs/domain"
)

func str(s string) *string { return &s }
func num(n int) *int       { return &n }

// A finished run to grade against: build and test run side by side on two
// runners, test restores a cache, deploy waits for both.
func graded() *domain.RunResult {
	return &domain.RunResult{
		RunIndex:     2,
		Status:       domain.RunSuccess,
		TotalSeconds: 200,
		Jobs: []domain.RunJob{
			{
				Name: "build", Runner: 0, Start: 0, End: 65, Status: domain.JobSuccess,
				Steps: []domain.RunStep{{Uses: "npm-build", Start: 0, End: 65, Status: domain.JobSuccess}},
			},
			{
				Name: "test", Runner: 1, Start: 0, End: 128, Status: domain.JobSuccess,
				Steps: []domain.RunStep{{
					Uses: "npm-ci", Start: 0, End: 8, Status: domain.JobSuccess,
					Cached: true, CacheKey: "node_modules",
				}},
			},
			{
				Name: "deploy", Runner: 0, Start: 128, End: 200, Status: domain.JobSuccess,
				Steps: []domain.RunStep{{Uses: "deploy", Start: 128, End: 200, Status: domain.JobSuccess}},
			},
			{
				Name: "migrate", Runner: 1, Start: 128, End: 148, Status: domain.JobFailed,
				Reason: "migrate hỏng",
				Steps: []domain.RunStep{{
					Uses: "migrate", Start: 128, End: 148, Status: domain.JobFailed,
				}},
			},
			{
				// Chưa từng chạy vì migrate hỏng — runner -1 và không có step nào.
				Name: "announce", Runner: -1, Start: 148, End: 148, Status: domain.JobSkipped,
				Reason: "migrate hỏng", Steps: []domain.RunStep{},
			},
		},
	}
}

func TestEval(t *testing.T) {
	cases := []struct {
		name string
		goal domain.Goal
		want bool
	}{
		{"run succeeded", domain.Goal{All: []domain.Predicate{{RunStatus: str("success")}}}, true},
		{"run failed", domain.Goal{All: []domain.Predicate{{RunStatus: str("failed")}}}, false},

		{"inside the budget", domain.Goal{All: []domain.Predicate{{TotalSecondsLTE: num(200)}}}, true},
		{"over the budget", domain.Goal{All: []domain.Predicate{{TotalSecondsLTE: num(199)}}}, false},

		{"job is there", domain.Goal{All: []domain.Predicate{{JobPresent: str("deploy")}}}, true},
		{"job is not", domain.Goal{All: []domain.Predicate{{JobPresent: str("lint")}}}, false},

		{"cache was restored", domain.Goal{All: []domain.Predicate{{CacheHit: str("node_modules")}}}, true},
		{"a different cache", domain.Goal{All: []domain.Predicate{{CacheHit: str("docker-layers")}}}, false},

		{
			"two jobs overlapped",
			domain.Goal{All: []domain.Predicate{{JobsParallel: []string{"build", "test"}}}},
			true,
		},
		{
			// deploy starts at 128, exactly when test ends. A handover on one runner
			// is the arrangement this clause exists to reject.
			"one starts as the other ends",
			domain.Goal{All: []domain.Predicate{{JobsParallel: []string{"test", "deploy"}}}},
			false,
		},
		{
			"a job that never ran",
			domain.Goal{All: []domain.Predicate{{JobsParallel: []string{"build", "lint"}}}},
			false,
		},

		{
			"a job that failed",
			domain.Goal{All: []domain.Predicate{
				{JobStatus: &domain.JobStatusClause{Job: "migrate", Status: "failed"}},
			}},
			true,
		},
		{
			// Cả bài học fail-fast nằm ở chỗ phân biệt được hai cái này: job vỡ và
			// job chưa từng được thử là hai chuyện khác nhau.
			"a job that was skipped, not failed",
			domain.Goal{All: []domain.Predicate{
				{JobStatus: &domain.JobStatusClause{Job: "announce", Status: "skipped"}},
			}},
			true,
		},
		{
			"skipped is not failed",
			domain.Goal{All: []domain.Predicate{
				{JobStatus: &domain.JobStatusClause{Job: "announce", Status: "failed"}},
			}},
			false,
		},
		{
			"a job that is not in the pipeline at all",
			domain.Goal{All: []domain.Predicate{
				{JobStatus: &domain.JobStatusClause{Job: "khong-co", Status: "success"}},
			}},
			false,
		},
		{
			"every clause holds",
			domain.Goal{All: []domain.Predicate{
				{RunStatus: str("success")},
				{TotalSecondsLTE: num(240)},
				{CacheHit: str("node_modules")},
				{JobsParallel: []string{"build", "test"}},
			}},
			true,
		},
		{
			"one clause out of four fails the lot",
			domain.Goal{All: []domain.Predicate{
				{RunStatus: str("success")},
				{TotalSecondsLTE: num(10)},
			}},
			false,
		},
		{
			"two conditions in one clause",
			domain.Goal{All: []domain.Predicate{{RunStatus: str("success"), TotalSecondsLTE: num(200)}}},
			true,
		},
	}

	r := graded()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Eval(&c.goal, r)
			if err != nil {
				t.Fatalf("Eval: %v", err)
			}
			if got != c.want {
				t.Errorf("Eval = %v, want %v", got, c.want)
			}
		})
	}
}

// An empty goal is what a half-written task looks like in the database, and it
// would pass every student including one whose pipeline never started.
func TestEvalRefusesAnEmptyGoal(t *testing.T) {
	for _, c := range []struct {
		name string
		goal *domain.Goal
	}{
		{"nil", nil},
		{"no clauses", &domain.Goal{}},
		{"a clause that asks nothing", &domain.Goal{All: []domain.Predicate{{}}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			passed, err := Eval(c.goal, graded())
			if !errors.Is(err, domain.ErrEmptyGoal) {
				t.Fatalf("error = %v, want ErrEmptyGoal", err)
			}
			if passed {
				t.Error("Eval passed the student anyway")
			}
		})
	}
}
