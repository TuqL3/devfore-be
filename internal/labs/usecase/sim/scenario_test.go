package sim

import (
	"errors"
	"fmt"
	"testing"

	"github.com/devforge/be/internal/labs/domain"
)

func minimalScenario() *domain.Scenario {
	return &domain.Scenario{
		Version:     1,
		RunnerCount: 2,
		Catalog:     map[string]domain.ScenarioStep{"checkout": {Seconds: 5}},
	}
}

// Both callers hand this package a scenario somebody else chose the numbers in:
// the playground takes one straight off the request body, a lab reads one an
// author saved. The scheduler allocates a slot per runner before it reads a job,
// which makes runner_count the field where a large value is not a slow answer but
// an out-of-memory kill.
func TestCheckScenarioBounds(t *testing.T) {
	cases := []struct {
		name     string
		mut      func(*domain.Scenario)
		rejected bool
	}{
		{"ok", func(*domain.Scenario) {}, false},
		{"no catalog", func(s *domain.Scenario) { s.Catalog = nil }, true},
		{"runners zero", func(s *domain.Scenario) { s.RunnerCount = 0 }, true},
		{"runners at the ceiling", func(s *domain.Scenario) {
			s.RunnerCount = MaxScenarioRunners
		}, false},
		{"runners over the ceiling", func(s *domain.Scenario) {
			s.RunnerCount = MaxScenarioRunners + 1
		}, true},
		{"runners absurd", func(s *domain.Scenario) { s.RunnerCount = 2_000_000_000 }, true},
		{"catalog too big", func(s *domain.Scenario) {
			for i := 0; i <= MaxScenarioSteps; i++ {
				s.Catalog[fmt.Sprintf("step-%d", i)] = domain.ScenarioStep{Seconds: 1}
			}
		}, true},
		{"step too long", func(s *domain.Scenario) {
			s.Catalog["checkout"] = domain.ScenarioStep{Seconds: MaxStepSeconds + 1}
		}, true},
		{"step at the ceiling", func(s *domain.Scenario) {
			s.Catalog["checkout"] = domain.ScenarioStep{Seconds: MaxStepSeconds}
		}, false},
		{"negative restore", func(s *domain.Scenario) { s.CacheRestoreSeconds = -1 }, true},
		{"flaky over 100", func(s *domain.Scenario) {
			s.Catalog["checkout"] = domain.ScenarioStep{Seconds: 5, Flaky: 101}
		}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := minimalScenario()
			tc.mut(sc)
			err := CheckScenario(sc)
			if tc.rejected && !errors.Is(err, domain.ErrInvalidScenario) {
				t.Fatalf("want ErrInvalidScenario, got %v", err)
			}
			if !tc.rejected && err != nil {
				t.Fatalf("want accepted, got %v", err)
			}
		})
	}
}

// Two broken steps, and the author is told about the same one every time. Map
// order would otherwise make one mistake look like two.
func TestCheckScenarioNamesTheSameStepEveryTime(t *testing.T) {
	sc := minimalScenario()
	sc.Catalog["aaa"] = domain.ScenarioStep{Seconds: -1}
	sc.Catalog["zzz"] = domain.ScenarioStep{Seconds: -1}

	first := CheckScenario(sc).Error()
	for i := 0; i < 20; i++ {
		if got := CheckScenario(sc).Error(); got != first {
			t.Fatalf("message changed between calls:\n%q\n%q", first, got)
		}
	}
}

// The scenarios the real labs and the playground ship have to pass their own
// gate. A bound nothing in the product can satisfy is a bound set wrong.
func TestShippedScenarioShapePasses(t *testing.T) {
	if err := CheckScenario(scenario()); err != nil {
		t.Fatalf("the test scenario is rejected by its own bounds: %v", err)
	}
}
