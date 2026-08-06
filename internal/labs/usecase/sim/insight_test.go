package sim

import (
	"strings"
	"testing"

	"github.com/devforge/be/internal/labs/domain"
)

// run is the whole path a student takes: text in, schedule and observations out.
// The rules are tested through it rather than called directly, because every one
// of them reads a field the engine fills in, and a rule that is right about a
// hand-written RunResult the engine would never produce is right about nothing.
func run(t *testing.T, sc *domain.Scenario, src string) *domain.RunResult {
	t.Helper()
	return Run(sc, mustParse(t, sc, src), "test", 1, nil)
}

func hasInsight(r *domain.RunResult, substr string) bool {
	for _, line := range r.Insights {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}

// The line that answers "which job do I attack". It has to name the chain and
// account for the jobs it rules out, since ruling them out is the only part a
// student cannot read off the chart themselves.
func TestCriticalChainNamesTheChainAndWhatItRulesOut(t *testing.T) {
	sc := scenario()
	// lint is short and off the chain; build → deploy sets the clock.
	r := run(t, sc, `
jobs:
  build:
    steps: [checkout, npm-build]
  deploy:
    needs: [build]
    steps: [checkout, docker-build]
  lint:
    steps: [checkout]
`)
	if sc.RunnerCount != 2 {
		t.Fatalf("scenario changed: want 2 runners, got %d", sc.RunnerCount)
	}
	if !hasInsight(r, "đường găng là build → deploy") {
		t.Fatalf("no critical-chain insight: %#v", r.Insights)
	}
	if !hasInsight(r, "1 job ngoài nó") {
		t.Fatalf("chain insight did not count the off-chain job: %#v", r.Insights)
	}
}

// The claim "speeding up an off-chain job changes nothing" stops being true the
// moment a job had to queue, because finishing early frees a machine early. The
// rule has to go quiet there, and hand the run to runnerQueue instead.
func TestCriticalChainSilentWhenAJobQueued(t *testing.T) {
	sc := scenario()
	sc.RunnerCount = 1
	r := run(t, sc, `
jobs:
  build:
    steps: [checkout, npm-build]
  test:
    steps: [checkout, npm-test]
  lint:
    steps: [checkout]
`)
	if hasInsight(r, "đường găng") {
		t.Fatalf("critical-chain fired on a queued run: %#v", r.Insights)
	}
	if !hasInsight(r, "mới tới lượt") {
		t.Fatalf("no runner-queue insight: %#v", r.Insights)
	}
	if !hasInsight(r, "chỉ có 1 runner") {
		t.Fatalf("queue insight did not name the fleet size: %#v", r.Insights)
	}
}

// Nothing queues when the fleet is big enough, and a rule that says otherwise
// would teach a student to buy runners they already have.
func TestRunnerQueueSilentWhenFleetIsEnough(t *testing.T) {
	sc := scenario()
	sc.RunnerCount = 3
	r := run(t, sc, `
jobs:
  build:
    steps: [checkout, npm-build]
  test:
    steps: [checkout, npm-test]
  lint:
    steps: [checkout]
`)
	if hasInsight(r, "mới tới lượt") {
		t.Fatalf("runner-queue fired with a runner to spare: %#v", r.Insights)
	}
}

// Waiting on needs is not waiting on the fleet. A job that starts the instant its
// dependency ends waited for the dependency, and calling that a queue would point
// at the machine count when the answer is the needs line.
func TestRunnerQueueIgnoresWaitingOnNeeds(t *testing.T) {
	sc := scenario()
	sc.RunnerCount = 2
	r := run(t, sc, `
jobs:
  build:
    steps: [checkout, npm-build]
  deploy:
    needs: [build]
    steps: [checkout, docker-build]
`)
	if hasInsight(r, "mới tới lượt") {
		t.Fatalf("runner-queue counted a needs wait: %#v", r.Insights)
	}
}

// A misspelled cache key is accepted by the parser on purpose — the key is a name
// the author invents, so there is nothing to spell-check it against — which makes
// this the only place a student hears that the line does nothing.
func TestDeadCacheKeyIsReported(t *testing.T) {
	sc := scenario()
	r := run(t, sc, `
jobs:
  build:
    steps: [checkout, npm-ci, npm-build]
    cache: [node-modules]
`)
	if !hasInsight(r, `khai cache "node-modules"`) {
		t.Fatalf("no dead-cache-key insight: %#v", r.Insights)
	}
}

func TestDeadCacheKeySilentOnTheRightKey(t *testing.T) {
	sc := scenario()
	r := run(t, sc, `
jobs:
  build:
    steps: [checkout, npm-ci, npm-build]
    cache: [node_modules]
`)
	if hasInsight(r, "khai cache") {
		t.Fatalf("dead-cache-key fired on a key that is used: %#v", r.Insights)
	}
}

// The cap cuts from the end, so the ordering in insights() is load-bearing: the
// one-line rules have to survive a pipeline that is wrong in ten per-job ways.
func TestOneLineInsightsSurviveTheCap(t *testing.T) {
	sc := scenario()
	sc.RunnerCount = 1
	var b strings.Builder
	b.WriteString("jobs:\n")
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		b.WriteString("  " + name + ":\n")
		b.WriteString("    steps: [checkout, npm-ci]\n")
		b.WriteString("    cache: [typo]\n")
	}
	r := run(t, sc, b.String())

	if len(r.Insights) != maxInsights+1 {
		t.Fatalf("want %d lines plus the overflow note, got %d: %#v",
			maxInsights+1, len(r.Insights), r.Insights)
	}
	if !hasInsight(r, "mới tới lượt") {
		t.Fatalf("the one-line queue insight was cut: %#v", r.Insights)
	}
}

// Determinism, for the new rules specifically. Two of them walk r.Jobs and pick a
// maximum, and a tie broken by map order would make a stored run and its re-grade
// disagree about what it said.
func TestInsightsAreDeterministic(t *testing.T) {
	sc := scenario()
	sc.RunnerCount = 1
	src := `
jobs:
  alpha:
    steps: [checkout, npm-test]
  beta:
    steps: [checkout, npm-test]
  gamma:
    steps: [checkout, npm-test]
`
	first := run(t, sc, src).Insights
	for i := 0; i < 20; i++ {
		next := run(t, sc, src).Insights
		if len(next) != len(first) {
			t.Fatalf("insight count changed between runs: %d vs %d", len(first), len(next))
		}
		for j := range first {
			if next[j] != first[j] {
				t.Fatalf("insight %d changed between runs:\n%q\n%q", j, first[j], next[j])
			}
		}
	}
}
