// Package sim runs a student's CI/CD pipeline against an authored scenario and
// says what would have happened. Nothing here touches Docker, the network or the
// clock: a run is a pure function of the scenario, the pipeline text, the session
// and the run number, which is what lets a stored result be graded now and
// re-graded later without the engine having to agree with its past self by luck.
package sim

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/devforge/be/internal/labs/domain"
)

// Bounds on what a student may hand in. Not a defence against an attacker — the
// route is behind a live session of their own — but against an author's runaway
// example and a client that resends in a loop.
const (
	MaxPipelineBytes = 16 << 10
	MaxJobs          = 20
	MaxStepsPerJob   = 30
)

// ErrPipelineInvalid wraps every rejection from Parse, so a caller can map the
// whole class to one status code while still showing the student the specific
// sentence underneath it. The sentences are in Vietnamese because they are shown
// beside the editor rather than logged: they are the lesson, not a diagnostic.
var ErrPipelineInvalid = errors.New("pipeline không hợp lệ")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrPipelineInvalid, fmt.Sprintf(format, args...))
}

// Parse turns the YAML a student wrote into a graph the engine can run, and
// refuses anything the engine could not. Every rejection here is a sentence they
// read instead of a run they cannot explain, so the reasons name the job, the
// step or the key at fault rather than reporting that something was wrong.
func Parse(src string, sc *domain.Scenario) (*domain.Pipeline, error) {
	if len(sc.Catalog) == 0 {
		return nil, domain.ErrEmptyScenario
	}
	if strings.TrimSpace(src) == "" {
		return nil, invalid("pipeline đang trống")
	}
	if len(src) > MaxPipelineBytes {
		return nil, invalid("pipeline dài hơn %d byte cho phép", MaxPipelineBytes)
	}

	var p domain.Pipeline
	// Strict on purpose. A misspelled key silently ignored is the worst outcome
	// available here: the student changes a line, nothing about the run changes,
	// and the lesson they draw is that the line did not matter.
	if err := yaml.UnmarshalWithOptions([]byte(src), &p, yaml.Strict()); err != nil {
		return nil, invalid("%s", firstLine(err.Error()))
	}

	if len(p.Jobs) == 0 {
		return nil, invalid("chưa khai job nào")
	}
	if len(p.Jobs) > MaxJobs {
		return nil, invalid("có %d job, nhiều hơn mức %d cho phép", len(p.Jobs), MaxJobs)
	}

	for _, name := range JobNames(&p) {
		job := p.Jobs[name]
		if strings.TrimSpace(name) == "" {
			return nil, invalid("có một job không có tên")
		}
		if len(job.Steps) == 0 {
			return nil, invalid("job %q không có step nào", name)
		}
		if len(job.Steps) > MaxStepsPerJob {
			return nil, invalid("job %q có %d step, nhiều hơn mức %d cho phép",
				name, len(job.Steps), MaxStepsPerJob)
		}
		for _, uses := range job.Steps {
			if _, ok := sc.Catalog[uses]; !ok {
				return nil, invalid("job %q dùng step %q, bài lab này không có step đó",
					name, uses)
			}
		}
		seen := make(map[string]bool, len(job.Needs))
		for _, need := range job.Needs {
			switch {
			case need == name:
				return nil, invalid("job %q needs chính nó", name)
			case seen[need]:
				return nil, invalid("job %q khai %q hai lần trong needs", name, need)
			}
			if _, ok := p.Jobs[need]; !ok {
				return nil, invalid("job %q needs %q, ở đây không có job tên đó", name, need)
			}
			seen[need] = true
		}
	}

	if cycle := findCycle(&p); cycle != nil {
		return nil, invalid("các job này chờ lẫn nhau: %s", strings.Join(cycle, " → "))
	}
	return &p, nil
}

// JobNames is every job in a stable order. Go randomises map iteration and the
// engine schedules ties by the order it sees them, so sorting here is what makes
// two runs of the same pipeline identical rather than usually identical.
func JobNames(p *domain.Pipeline) []string {
	names := make([]string, 0, len(p.Jobs))
	for name := range p.Jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// findCycle returns one cycle as a readable chain, or nil when the graph is
// acyclic. A cycle has to be caught here rather than in the engine: the scheduler
// would sit waiting for a job that can never become ready, and "nothing ran" is
// not a sentence anybody can act on.
func findCycle(p *domain.Pipeline) []string {
	const (
		unvisited = 0
		active    = 1
		done      = 2
	)
	state := make(map[string]int, len(p.Jobs))
	var stack []string

	var walk func(string) []string
	walk = func(name string) []string {
		state[name] = active
		stack = append(stack, name)
		for _, need := range p.Jobs[name].Needs {
			switch state[need] {
			case active:
				// The cycle is the tail of the stack from where this name first
				// appears, closed by naming it again.
				for i, n := range stack {
					if n == need {
						return append(append([]string{}, stack[i:]...), need)
					}
				}
			case unvisited:
				if cycle := walk(need); cycle != nil {
					return cycle
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[name] = done
		return nil
	}

	for _, name := range JobNames(p) {
		if state[name] == unvisited {
			if cycle := walk(name); cycle != nil {
				return cycle
			}
		}
	}
	return nil
}

// firstLine keeps the parser's own message to its first line. goccy prints a
// source excerpt under the error, which is useful in a terminal and noise in a
// toast next to the editor the student is already looking at.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
