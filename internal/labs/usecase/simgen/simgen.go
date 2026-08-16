// Package simgen turns a sentence into a scenario the engine will actually run.
//
// The model is never trusted with the answer. What it produces goes through
// `sim.CheckScenario` and then through `sim.Parse` on every example it wrote,
// using the same engine a student's pipeline runs on. A scenario that fails
// either is handed back to the model with the exact error, once. Only something
// that survives both reaches the client.
//
// That loop is the whole point. Without it this is a JSON generator that is
// right most of the time, and "most of the time" is indistinguishable from a
// broken simulator to somebody who is still learning what a pipeline is.
package simgen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase/sim"
)

// Bounds on one request. The prompt cap is generous for a sentence describing a
// stack and mean for anything trying to smuggle a document through; the turn cap
// keeps a long conversation from re-sending an unbounded history every time.
const (
	MaxPromptChars = 2000
	MaxTurns       = 12
	// One repair round, not a loop. If the model cannot produce a runnable
	// scenario from an explicit engine error the second time, a third attempt is
	// paying again for the same misunderstanding.
	maxAttempts = 2
)

var (
	ErrEmptyPrompt = errors.New("no request was entered")
	ErrPromptLong  = fmt.Errorf("the request is longer than the %d characters allowed", MaxPromptChars)
	// ErrUnusable is the model failing to produce something the engine accepts
	// even after being told exactly what was wrong. Distinct from a transport
	// error: nothing is down, the answer is just not usable.
	ErrUnusable = errors.New("could not build a scenario that runs")
	// ErrDisabled is this deployment having no API key. Owned here rather than
	// re-exported from the adapter so the usecase can refuse before it charges,
	// without importing the transport.
	ErrDisabled = errors.New("the feature is not enabled")
)

// Generator is the model, as much of it as this package needs.
//
// Enabled is separate from JSON so a deployment with no API key can be turned
// away before the quota is spent. Without it a switched-off feature silently
// eats a user's daily budget on every 503, and the day the key is finally added
// they have none left.
type Generator interface {
	Enabled() bool
	JSON(ctx context.Context, system string, turns []domain.AITurn, schema map[string]any) (json.RawMessage, error)
}

// Turn is the domain's conversation turn, re-exported so a caller assembling one
// does not have to reach past this package for the type.
type Turn = domain.AITurn

// Quota is the per-user daily cap. Every generation costs real money, so this is
// not optional: without it one loop in a client is an unbounded bill.
type Quota interface {
	Take(ctx context.Context, userID int64) error
}

type Simgen struct {
	gen   Generator
	quota Quota
}

func New(gen Generator, quota Quota) *Simgen { return &Simgen{gen: gen, quota: quota} }

// Input is one press of the button: the new instruction, plus whatever came
// before it in this conversation.
type Input struct {
	Prompt string
	// History alternates user and assistant turns. Assistant turns carry the
	// JSON produced last time, which is what makes "change runners to 4" mean
	// something rather than starting over.
	History []Turn
}

// Result is what the client gets: a scenario the engine has already run, plus
// the model's own one-line account of what it built.
type Result struct {
	Scenario *domain.Scenario `json:"scenario"`
	Notes    string           `json:"notes"`
	// How many model calls this took. Two means the first attempt did not run
	// and the engine's error fixed it — worth surfacing rather than hiding,
	// because it is the difference between a lucky answer and a checked one.
	Attempts int `json:"attempts"`
}

func (s *Simgen) Generate(ctx context.Context, userID int64, in Input) (*Result, error) {
	prompt := strings.TrimSpace(in.Prompt)
	switch {
	case prompt == "":
		return nil, ErrEmptyPrompt
	case len(prompt) > MaxPromptChars:
		return nil, ErrPromptLong
	case !s.gen.Enabled():
		// Ask before charging. The generator would refuse anyway, but by then
		// the quota is gone.
		return nil, ErrDisabled
	}
	if err := s.quota.Take(ctx, userID); err != nil {
		return nil, err
	}

	turns := trimHistory(in.History)
	turns = append(turns, Turn{Text: prompt})

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		raw, err := s.gen.JSON(ctx, systemPrompt, turns, outputSchema())
		if err != nil {
			return nil, err
		}

		sc, notes, err := decode(raw)
		if err == nil {
			err = verify(sc)
		}
		if err == nil {
			return &Result{Scenario: sc, Notes: notes, Attempts: attempt}, nil
		}
		lastErr = err

		// Hand back the engine's own sentence, not a paraphrase. Those messages
		// name the job, step or key at fault because they were written for a
		// person to act on, and the model acts on them the same way.
		turns = append(turns,
			Turn{Assistant: true, Text: string(raw)},
			Turn{Text: "The scenario above does not run. The engine reports: " + err.Error() +
				"\nFix it and return the complete scenario."},
		)
	}
	return nil, fmt.Errorf("%w: %v", ErrUnusable, lastErr)
}

// trimHistory keeps the tail. The opening instruction matters less than the last
// few corrections, and an unbounded history is an unbounded bill.
func trimHistory(h []Turn) []Turn {
	if len(h) <= MaxTurns {
		return append([]Turn{}, h...)
	}
	return append([]Turn{}, h[len(h)-MaxTurns:]...)
}

// verify is the part that makes this trustworthy: the engine, not the model,
// decides whether the scenario is usable.
func verify(sc *domain.Scenario) error {
	if err := sim.CheckScenario(sc); err != nil {
		return err
	}
	if len(sc.Examples) == 0 {
		return errors.New("the scenario has no examples")
	}
	for _, ex := range sc.Examples {
		// Parsed, not run to green. An example that ends red on purpose — a
		// migration that always fails, a flaky step — is a legitimate lesson;
		// an example that does not parse is a broken button.
		if _, err := sim.Parse(ex.Pipeline, sc); err != nil {
			return fmt.Errorf("example %q: %w", ex.Title, err)
		}
	}
	return nil
}
