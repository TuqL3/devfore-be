package sim

import (
	"fmt"
	"sort"

	"github.com/devforge/be/internal/labs/domain"
)

// Bounds on the authored half of a run. Two callers hand a scenario to this
// package — the playground takes one from the request body, a lab reads one out
// of `labs.sim_scenario` — and only one of them is a number somebody typed into
// an admin form. Checking both anyway is the cheaper arrangement: the reason a
// bad scenario is dangerous is what the engine does with it, and that is the same
// engine either way.
//
// MaxScenarioRunners is the one bound that is not about taste. The scheduler
// allocates a slot per runner before it reads a single job, so a scenario
// claiming two billion runners asks this process for sixteen gigabytes and is
// killed rather than answered. Sixteen is generous — a fleet teaches what it has
// to teach long before then — and it is a ceiling, not a value: a scenario asking
// for two still gets two.
const (
	MaxScenarioRunners = 16
	MaxScenarioSteps   = 64
	MaxStepSeconds     = 24 * 60 * 60
)

// CheckScenario refuses a scenario the engine could only answer nonsense for, or
// could not survive. Nonsense is the worse of the two: it is indistinguishable
// from a bug in the simulator, and a student who suspects the simulator has
// stopped learning from it.
func CheckScenario(sc *domain.Scenario) error {
	switch {
	case len(sc.Catalog) == 0:
		return fmt.Errorf("%w: kịch bản chưa có step nào trong catalog", domain.ErrInvalidScenario)
	case len(sc.Catalog) > MaxScenarioSteps:
		return fmt.Errorf("%w: catalog có %d step, nhiều hơn mức %d cho phép",
			domain.ErrInvalidScenario, len(sc.Catalog), MaxScenarioSteps)
	case sc.RunnerCount < 1:
		return fmt.Errorf("%w: runner_count phải từ 1 trở lên", domain.ErrInvalidScenario)
	case sc.RunnerCount > MaxScenarioRunners:
		return fmt.Errorf("%w: runner_count là %d, nhiều hơn mức %d cho phép",
			domain.ErrInvalidScenario, sc.RunnerCount, MaxScenarioRunners)
	case sc.CacheRestoreSeconds < 0:
		return fmt.Errorf("%w: cache_restore_seconds âm", domain.ErrInvalidScenario)
	}
	// Sorted, so an author with two broken steps is told about the same one every
	// time they press save rather than whichever the map handed over first.
	for _, name := range catalogNames(sc) {
		step := sc.Catalog[name]
		switch {
		case step.Seconds < 0:
			return fmt.Errorf("%w: step %q có seconds âm", domain.ErrInvalidScenario, name)
		case step.Seconds > MaxStepSeconds:
			return fmt.Errorf("%w: step %q dài %ds, quá mức %ds cho phép",
				domain.ErrInvalidScenario, name, step.Seconds, MaxStepSeconds)
		case step.Flaky < 0 || step.Flaky > 100:
			return fmt.Errorf("%w: step %q có flaky ngoài khoảng 0..100",
				domain.ErrInvalidScenario, name)
		}
	}
	return nil
}

// catalogNames is every step in the catalogue, sorted. Go randomises map
// iteration, and an error message that names a different step each time reads as
// two bugs instead of one.
func catalogNames(sc *domain.Scenario) []string {
	names := make([]string, 0, len(sc.Catalog))
	for name := range sc.Catalog {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
