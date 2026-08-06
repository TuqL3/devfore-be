package sim

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/devforge/be/internal/labs/domain"
)

// A scenario shaped like the node pipeline the first lab will use. Written once
// here so a test that changes a number changes it on purpose.
func scenario() *domain.Scenario {
	return &domain.Scenario{
		Version:             1,
		RunnerCount:         2,
		CacheRestoreSeconds: 8,
		Catalog: map[string]domain.ScenarioStep{
			"checkout":     {Seconds: 5},
			"npm-ci":       {Seconds: 90, Cacheable: "node_modules"},
			"npm-test":     {Seconds: 120},
			"npm-build":    {Seconds: 60, Produces: "dist"},
			"docker-build": {Seconds: 180, Consumes: "dist"},
			"e2e":          {Seconds: 200, Flaky: 50},
			"always-red":   {Seconds: 30, Flaky: 100},
		},
	}
}

func mustParse(t *testing.T, sc *domain.Scenario, src string) *domain.Pipeline {
	t.Helper()
	p, err := Parse(src, sc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return p
}

func job(r *domain.RunResult, name string) *domain.RunJob {
	for i := range r.Jobs {
		if r.Jobs[i].Name == name {
			return &r.Jobs[i]
		}
	}
	return nil
}

// The one property everything else rests on. A run is stored once and graded
// afterwards, possibly long afterwards: if the engine could return two answers
// for the same input, a report and the score next to it would drift apart with
// nobody able to say which was right.
func TestRunIsDeterministic(t *testing.T) {
	sc := scenario()
	p := mustParse(t, sc, `
jobs:
  build:
    steps: [checkout, npm-build]
    cache: [node_modules]
  test:
    needs: [build]
    steps: [checkout, e2e]
  ship:
    needs: [build]
    steps: [docker-build]
`)

	first := Run(sc, p, "session-abc", 3, []string{"node_modules"})
	second := Run(sc, p, "session-abc", 3, []string{"node_modules"})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("two runs of the same pipeline differ:\n%+v\n%+v", first, second)
	}
}

// The lesson the simulator exists to teach. If this breaks, the lab is telling
// students the opposite of the truth about their pipelines.
func TestParallelJobsShortenTheRun(t *testing.T) {
	sc := scenario()

	serial := Run(sc, mustParse(t, sc, `
jobs:
  build:
    steps: [checkout, npm-build]
  test:
    needs: [build]
    steps: [checkout, npm-test]
`), "s", 1, nil)

	parallel := Run(sc, mustParse(t, sc, `
jobs:
  build:
    steps: [checkout, npm-build]
  test:
    steps: [checkout, npm-test]
`), "s", 1, nil)

	if serial.TotalSeconds != 190 {
		t.Errorf("serial total = %d, want 190 (65 then 125)", serial.TotalSeconds)
	}
	if parallel.TotalSeconds != 125 {
		t.Errorf("parallel total = %d, want 125 (the longer of the two)", parallel.TotalSeconds)
	}
	if parallel.Jobs[0].Runner == parallel.Jobs[1].Runner {
		t.Errorf("both jobs landed on runner %d, expected one each", parallel.Jobs[0].Runner)
	}
	if got := serial.CriticalPath; !reflect.DeepEqual(got, []string{"build", "test"}) {
		t.Errorf("critical path = %v, want [build test]", got)
	}
}

// A fleet the pipeline never uses is a fleet nobody is paying attention to.
func TestOneRunnerSerialisesIndependentJobs(t *testing.T) {
	sc := scenario()
	sc.RunnerCount = 1

	r := Run(sc, mustParse(t, sc, `
jobs:
  build:
    steps: [checkout, npm-build]
  test:
    steps: [checkout, npm-test]
`), "s", 1, nil)

	if r.TotalSeconds != 190 {
		t.Errorf("total = %d, want 190: one runner cannot overlap them", r.TotalSeconds)
	}
	if a, b := job(r, "build"), job(r, "test"); a.End > b.Start && b.End > a.Start {
		t.Errorf("jobs overlapped on a single runner: %+v %+v", a, b)
	}
}

func TestCacheIsWarmOnTheSecondRun(t *testing.T) {
	sc := scenario()
	p := mustParse(t, sc, `
jobs:
  build:
    steps: [checkout, npm-ci, npm-build]
    cache: [node_modules]
`)

	first := Run(sc, p, "s", 1, nil)
	if first.TotalSeconds != 155 {
		t.Fatalf("first run = %ds, want 155: nothing is cached yet", first.TotalSeconds)
	}
	if got := first.WarmCaches; !reflect.DeepEqual(got, []string{"node_modules"}) {
		t.Fatalf("warm caches after first run = %v, want [node_modules]", got)
	}

	second := Run(sc, p, "s", 2, first.WarmCaches)
	if second.TotalSeconds != 73 {
		t.Errorf("second run = %ds, want 73: npm-ci restores in 8s", second.TotalSeconds)
	}
	if step := job(second, "build").Steps[1]; !step.Cached || step.CacheKey != "node_modules" {
		t.Errorf("npm-ci was not recorded as a cache hit: %+v", step)
	}
}

// Forgetting the cache line in the second job is the mistake this whole model
// exists to make visible, so the discount has to stop at the job that asked.
func TestCacheOnlyHelpsTheJobThatAsksForIt(t *testing.T) {
	sc := scenario()
	p := mustParse(t, sc, `
jobs:
  build:
    steps: [npm-ci]
    cache: [node_modules]
  test:
    steps: [npm-ci]
`)

	r := Run(sc, p, "s", 2, []string{"node_modules"})
	if step := job(r, "build").Steps[0]; !step.Cached {
		t.Errorf("build asked for the cache and did not get it: %+v", step)
	}
	if step := job(r, "test").Steps[0]; step.Cached {
		t.Errorf("test never asked for the cache but got it: %+v", step)
	}
	if got := job(r, "test").End - job(r, "test").Start; got != 90 {
		t.Errorf("test took %ds, want the full 90", got)
	}
}

func TestMissingArtifactFailsTheJobAndSkipsWhatWaitsOnIt(t *testing.T) {
	sc := scenario()
	r := Run(sc, mustParse(t, sc, `
jobs:
  ship:
    steps: [docker-build]
  announce:
    needs: [ship]
    steps: [checkout]
`), "s", 1, nil)

	shipJob := job(r, "ship")
	if shipJob.Status != domain.JobFailed {
		t.Fatalf("ship status = %q, want failed: nothing produced dist", shipJob.Status)
	}
	if !strings.Contains(shipJob.Reason, "dist") {
		t.Errorf("ship reason = %q, want it to name the artifact", shipJob.Reason)
	}
	announce := job(r, "announce")
	if announce.Status != domain.JobSkipped {
		t.Errorf("announce status = %q, want skipped", announce.Status)
	}
	if announce.Runner != -1 {
		t.Errorf("announce holds runner %d despite never running", announce.Runner)
	}
	if r.Status != domain.RunFailed {
		t.Errorf("run status = %q, want failed", r.Status)
	}
}

// One job is one workspace, so a step consumes what an earlier step of the same
// job produced. Refusing that would call the commonest pipeline shape there is —
// build, then package what was built — impossible, and a lab teaching that would
// be teaching the opposite of how CI works.
func TestArtifactFromAnEarlierStepOfTheSameJob(t *testing.T) {
	sc := scenario()
	r := Run(sc, mustParse(t, sc, `
jobs:
  ship:
    steps: [npm-build, docker-build]
`), "s", 1, nil)

	shipJob := job(r, "ship")
	if shipJob.Status != domain.JobSuccess {
		t.Fatalf("ship status = %q, want success: npm-build left dist right there — %+v",
			shipJob.Status, shipJob.Steps)
	}
	if r.Status != domain.RunSuccess {
		t.Errorf("run status = %q, want success", r.Status)
	}
}

// The order still matters: consuming before anything produced it fails, because
// that is a step reading a directory that is not there yet.
func TestArtifactConsumedBeforeItIsProduced(t *testing.T) {
	sc := scenario()
	r := Run(sc, mustParse(t, sc, `
jobs:
  ship:
    steps: [docker-build, npm-build]
`), "s", 1, nil)

	if got := job(r, "ship").Status; got != domain.JobFailed {
		t.Errorf("ship status = %q, want failed: docker-build ran before npm-build", got)
	}
}

// An artifact built by a job that happens to run first is not on this runner's
// disk. Passing here would teach a pipeline that works by luck of scheduling.
func TestArtifactMustArriveThroughNeeds(t *testing.T) {
	sc := scenario()
	r := Run(sc, mustParse(t, sc, `
jobs:
  build:
    steps: [npm-build]
  ship:
    steps: [docker-build]
`), "s", 1, nil)

	if job(r, "build").Status != domain.JobSuccess {
		t.Fatalf("build should have succeeded: %+v", job(r, "build"))
	}
	if got := job(r, "ship").Status; got != domain.JobFailed {
		t.Errorf("ship status = %q, want failed: it never declared needs on build", got)
	}
}

func TestFlakyStepFailsTheJobAndCostsItsTime(t *testing.T) {
	sc := scenario()
	r := Run(sc, mustParse(t, sc, `
jobs:
  test:
    steps: [checkout, always-red]
`), "s", 1, nil)

	testJob := job(r, "test")
	if testJob.Status != domain.JobFailed {
		t.Fatalf("status = %q, want failed", testJob.Status)
	}
	step := testJob.Steps[1]
	if !step.Flaky || step.Status != domain.JobFailed {
		t.Errorf("step not recorded as a flaky failure: %+v", step)
	}
	// 5 for checkout plus the full 30 the failing step still burned.
	if testJob.End != 35 {
		t.Errorf("job ended at %ds, want 35: a failing step is not free", testJob.End)
	}
}

// A retry has to be able to come out differently, or the lab teaches that flaky
// steps are deterministic — which is the belief that makes people retry forever.
func TestFlakyOutcomeVariesWithTheRunNumber(t *testing.T) {
	sc := scenario()
	p := mustParse(t, sc, `
jobs:
  test:
    steps: [e2e]
`)

	seen := map[string]bool{}
	for runIndex := 1; runIndex <= 8; runIndex++ {
		seen[Run(sc, p, "s", runIndex, nil).Status] = true
	}
	if len(seen) < 2 {
		t.Errorf("eight runs of a 50%% flaky step all came out %v", seen)
	}
}

func TestParseRejects(t *testing.T) {
	sc := scenario()

	cases := []struct {
		name string
		src  string
		want string
	}{
		// The wanted fragments are Vietnamese because the sentences are, and they
		// are Vietnamese because a student reads them beside the editor rather
		// than out of a log.
		{"empty", "   \n", "đang trống"},
		{"no jobs", "jobs: {}\n", "chưa khai job nào"},
		{"job with no steps", "jobs:\n  build:\n    steps: []\n", "không có step nào"},
		{
			"step outside the catalogue",
			"jobs:\n  build:\n    steps: [terraform-apply]\n",
			"không có step đó",
		},
		{
			"needs a job that is not there",
			"jobs:\n  build:\n    needs: [lint]\n    steps: [checkout]\n",
			"không có job tên đó",
		},
		{
			"needs itself",
			"jobs:\n  build:\n    needs: [build]\n    steps: [checkout]\n",
			"needs chính nó",
		},
		{
			"the same need twice",
			"jobs:\n  a:\n    steps: [checkout]\n  b:\n    needs: [a, a]\n    steps: [checkout]\n",
			"hai lần",
		},
		{
			"a cycle",
			"jobs:\n  a:\n    needs: [b]\n    steps: [checkout]\n  b:\n    needs: [a]\n    steps: [checkout]\n",
			"chờ lẫn nhau",
		},
		{
			"a key that does not exist",
			"jobs:\n  build:\n    stpes: [checkout]\n",
			"stpes",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.src, sc)
			if err == nil {
				t.Fatalf("Parse accepted it")
			}
			if !errors.Is(err, ErrPipelineInvalid) {
				t.Fatalf("error = %v, want it to wrap ErrPipelineInvalid", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), c.want)
			}
		})
	}
}

func TestParseRejectsAScenarioWithNoCatalogue(t *testing.T) {
	_, err := Parse("jobs:\n  build:\n    steps: [checkout]\n", &domain.Scenario{})
	if !errors.Is(err, domain.ErrEmptyScenario) {
		t.Fatalf("error = %v, want ErrEmptyScenario", err)
	}
}
