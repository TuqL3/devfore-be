package usecase

import (
	"context"
	"fmt"

	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase/sim"
)

// Bounds on what one playground request may claim about itself. Not a defence —
// the engine is pure arithmetic over a catalogue the caller supplied — but an
// unbounded run number or a thousand cache keys is a client bug, and a bug should
// not become a slow request.
//
// The scenario in the same body is bounded too, by sim.CheckScenario, which the
// graded path calls as well: what makes a scenario dangerous is what the engine
// does with it, and both paths run the same engine.
const (
	maxPlaygroundRun  = 200
	maxPlaygroundWarm = 32
)

// Playground runs a pipeline against a scenario the caller hands in whole.
// Nothing is read from the database and nothing is written to it.
//
// The scenarios live in the frontend's own source (`src/sims/`) rather than in a
// table, because their content *is* code: an admin form can only produce a shape
// it knows in advance, and the second kind of simulation has a different shape
// entirely. That decision is what removes every read from this path.
//
// It also means the caller supplies the scenario, the run number and the warm
// caches, and none of the three is verified. A client can invent a catalogue,
// claim a cache it never earned, or replay run 1 forever to dodge a flaky step.
// Nothing here is graded, so the only person a lie reaches is the one telling it.
//
// The lab path does the opposite for exactly that reason: there the scenario
// comes off `labs.sim_scenario` and the numbers off `sim_runs`, because there
// they decide a mark.
type Playground struct{}

func NewPlayground() *Playground { return &Playground{} }

// PreviewInput is one press of Run.
type PreviewInput struct {
	Pipeline string
	// Which press this is. Part of the flaky seed, so pressing Run again is a
	// genuinely different roll rather than the same one redrawn.
	Run int
	// Cache keys the caller says are warm from their previous run.
	Warm []string
}

func (p *Playground) Preview(
	_ context.Context, userID int64, sc *domain.Scenario, in PreviewInput,
) (*domain.RunResult, error) {
	if err := sim.CheckScenario(sc); err != nil {
		return nil, err
	}
	pipeline, err := sim.Parse(in.Pipeline, sc)
	if err != nil {
		return nil, err
	}
	// Seeded per player, so two people running the same pipeline can see
	// different flaky outcomes — which is the truth about flakiness — while one
	// person pressing Run twice on the same number sees the same one.
	seed := fmt.Sprintf("playground|%d", userID)
	return sim.Run(sc, pipeline, seed, clamp(in.Run), clampWarm(in.Warm)), nil
}

func clamp(n int) int {
	if n < 1 {
		return 1
	}
	if n > maxPlaygroundRun {
		return maxPlaygroundRun
	}
	return n
}

func clampWarm(keys []string) []string {
	if len(keys) > maxPlaygroundWarm {
		return keys[:maxPlaygroundWarm]
	}
	return keys
}
