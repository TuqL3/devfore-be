package simgen

import (
	"encoding/json"
	"fmt"

	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase/sim"
)

// The model writes an array of steps, not the map the engine stores. Structured
// outputs require `additionalProperties: false` on every object, which a map
// with author-invented keys cannot express — so the shape on the wire has names
// as a field and this file converts. Nothing is lost: the conversion is where a
// duplicate step name would be caught, and a map cannot hold one anyway.
type genStep struct {
	Name      string `json:"name"`
	Seconds   int    `json:"seconds"`
	Cacheable string `json:"cacheable"`
	Produces  string `json:"produces"`
	Consumes  string `json:"consumes"`
	Flaky     int    `json:"flaky"`
}

type genOutput struct {
	Notes               string                   `json:"notes"`
	RunnerCount         int                      `json:"runner_count"`
	CacheRestoreSeconds int                      `json:"cache_restore_seconds"`
	Steps               []genStep                `json:"steps"`
	Examples            []domain.ScenarioExample `json:"examples"`
}

func decode(raw json.RawMessage) (*domain.Scenario, string, error) {
	var out genOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, "", fmt.Errorf("the result is not readable JSON: %w", err)
	}

	sc := &domain.Scenario{
		Version:             1,
		RunnerCount:         out.RunnerCount,
		CacheRestoreSeconds: out.CacheRestoreSeconds,
		Catalog:             make(map[string]domain.ScenarioStep, len(out.Steps)),
		Examples:            out.Examples,
	}
	for _, st := range out.Steps {
		if st.Name == "" {
			return nil, "", fmt.Errorf("one step has no name")
		}
		if _, dup := sc.Catalog[st.Name]; dup {
			return nil, "", fmt.Errorf("step %q is declared twice", st.Name)
		}
		sc.Catalog[st.Name] = domain.ScenarioStep{
			Seconds:   st.Seconds,
			Cacheable: st.Cacheable,
			Produces:  st.Produces,
			Consumes:  st.Consumes,
			Flaky:     st.Flaky,
		}
	}
	return sc, out.Notes, nil
}

// outputSchema constrains generation rather than validating after it: the API
// will not emit anything that fails this, which is why there is no parse-retry
// layer anywhere in this package.
//
// The numeric limits are in the descriptions, not in the schema. Structured
// outputs do not support `minimum`/`maximum`, and duplicating them here would
// create a second copy of bounds `sim.CheckScenario` already owns — a copy that
// would silently drift the first time one of them changes.
func outputSchema() map[string]any {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required": []string{
			"notes", "runner_count", "cache_restore_seconds", "steps", "examples",
		},
		"properties": map[string]any{
			"notes": str("One or two sentences saying what this step catalog simulates " +
				"and the lesson that comes out of scheduling the jobs differently. Shown to the user."),
			"runner_count": map[string]any{
				"type": "integer",
				"description": fmt.Sprintf(
					"Maximum number of jobs running in parallel. From 1 to %d.", sim.MaxScenarioRunners),
			},
			"cache_restore_seconds": map[string]any{
				"type":        "integer",
				"description": "Seconds a step costs when restored from cache instead of redone. Usually 5-15.",
			},
			"steps": map[string]any{
				"type": "array",
				"description": fmt.Sprintf(
					"The pipeline steps that may be used. From 4 to %d steps.", sim.MaxScenarioSteps),
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required": []string{
						"name", "seconds", "cacheable", "produces", "consumes", "flaky",
					},
					"properties": map[string]any{
						"name": str("Step name, lowercase and hyphens, for example \"npm-ci\", " +
							"\"go-test\", \"docker-build\"."),
						"seconds": map[string]any{
							"type":        "integer",
							"description": "How long this step takes, in seconds. A realistic estimate.",
						},
						"cacheable": str("Name of the cache key this step leaves behind, for example \"node_modules\". " +
							"Empty string if the step leaves nothing worth caching."),
						"produces": str("Name of the artifact this step produces, for example \"dist\". " +
							"Empty string if it produces nothing."),
						"consumes": str("Name of the artifact this step requires. Empty string if it requires nothing. " +
							"The artifact must be produced by another step in this list."),
						"flaky": map[string]any{
							"type": "integer",
							"description": "Percentage of runs in which this step fails at random, 0 to 100. " +
								"Leave at 0 unless the point is to teach about flaky tests.",
						},
					},
				},
			},
			"examples": map[string]any{
				"type": "array",
				"description": "Two to four click-to-run example pipelines, ordered from the slow arrangement " +
					"to the fast one, so the learner has two numbers to compare.",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"title", "note", "pipeline"},
					"properties": map[string]any{
						"title": str("Short label on the button, for example \"① Sequential\"."),
						"note":  str("One line saying what this arrangement costs and why."),
						"pipeline": str("The YAML file contents, using \\n for line breaks. " +
							"Only these keys: jobs, needs, steps, cache."),
					},
				},
			},
		},
	}
}

// workedExample is one complete, runnable scenario, in exactly the shape the
// model must produce. It is the single strongest signal in the whole prompt:
// rules describe the engine, an example describes the *answer* — naming style,
// the magnitude of the seconds, how a note is phrased, how the examples build a
// comparison. Without one the model has never seen what "good" looks like.
//
// It is the Node.js CI scenario the frontend ships, which is authored content
// somebody wrote on purpose, not something invented for a prompt.
//
// `TestWorkedExampleIsRunnable` puts this through `decode` and `verify` — the
// same path the model's own output takes. An example that stops running is an
// example teaching the model to produce something the engine rejects, and that
// failure would otherwise be invisible until somebody read the generated output
// closely.
const workedExample = `{
  "notes": "The step catalog of a Node project: install, lint, test, build, package the image. Merged into one job everything waits its turn; split apart, two runners work in parallel.",
  "runner_count": 2,
  "cache_restore_seconds": 10,
  "steps": [
    {"name": "checkout", "seconds": 5, "cacheable": "", "produces": "", "consumes": "", "flaky": 0},
    {"name": "npm-ci", "seconds": 90, "cacheable": "node_modules", "produces": "", "consumes": "", "flaky": 0},
    {"name": "lint", "seconds": 25, "cacheable": "", "produces": "", "consumes": "", "flaky": 0},
    {"name": "npm-test", "seconds": 120, "cacheable": "", "produces": "", "consumes": "", "flaky": 0},
    {"name": "npm-build", "seconds": 60, "cacheable": "", "produces": "dist", "consumes": "", "flaky": 0},
    {"name": "docker-build", "seconds": 180, "cacheable": "", "produces": "", "consumes": "dist", "flaky": 0}
  ],
  "examples": [
    {
      "title": "\u2460 Sequential",
      "note": "one job does everything \u2014 every step waits for the last",
      "pipeline": "jobs:\n  ci:\n    steps: [checkout, npm-ci, npm-test, npm-build, docker-build]\n"
    },
    {
      "title": "\u2461 Parallel",
      "note": "split into two jobs, no needs \u2014 2 runners work at once",
      "pipeline": "jobs:\n  build:\n    steps: [checkout, npm-ci, npm-build]\n  test:\n    steps: [checkout, npm-ci, npm-test]\n"
    },
    {
      "title": "\u2462 Add cache",
      "note": "like \u2461 but declaring cache \u2014 press Run TWICE",
      "pipeline": "jobs:\n  build:\n    steps: [checkout, npm-ci, npm-build]\n    cache: [node_modules]\n  test:\n    steps: [checkout, npm-ci, npm-test]\n    cache: [node_modules]\n"
    }
  ]
}`

// systemPrompt is a constant, byte for byte, on every request — that is what
// makes the cache breakpoint on it worth having. Nothing per-user or per-time
// may be added here: either would turn every read into a fresh write.
const systemPrompt = `You write scenarios for a CI/CD pipeline simulator used to teach DevOps.

This engine does NOT run real commands. It only computes a schedule: which job
runs when, on which runner, for how long. Your job is to choose the step catalog
and the cost of each step so that the learner can see why one way of arranging
jobs is faster than another.

# The engine's complete rules

1. A job with no "needs" starts immediately. How many jobs run at once is capped
   by runner_count; the extras queue for a free runner.
2. "needs" means this job waits for that job to finish before it starts.
3. Steps within one job run sequentially, in the order written.
4. A step with "consumes" can only run if that artifact already exists: either
   produced by an earlier step IN THE SAME JOB, or by a job somewhere up its
   "needs" chain. A job running in parallel on another branch does not count —
   that runner's disk is not this one's.
5. Cache only goes warm ON THE NEXT RUN. The current run still pays full price.
   A job must declare "cache: [key]" itself to get the discount; forget the
   declaration and there is no discount.
6. When a step fails the job stops right there, and the jobs depending on it are
   skipped.

# Pipeline syntax, and that is all the keys there are

jobs:
  build:
    steps: [checkout, npm-ci, npm-build]
    cache: [node_modules]
  deploy:
    needs: [build]
    steps: [checkout, docker-build]

# Make it produce a lesson

The step catalog must allow at least one of the comparisons below, and the
examples must turn it into two different numbers:

- Everything merged into one job (slow) against split into parallel jobs (fast).
- Declaring cache against forgetting to declare it.
- One redundant "needs" line turning two parallel jobs into a sequence.
- More jobs than runners: splitting further stops helping.

That needs a few EXPENSIVE steps (60 seconds or more) — a pipeline of nothing
but 5-second steps schedules the same way whatever you do and teaches nothing.

# Scale

**5 to 8 steps.** The hard limit is 64, but that is a ceiling, not a target: a
20-job chart is unreadable, and the lesson dissolves into a pile of horizontal
bars. When somebody asks about "a system serving millions of users", the thing
to simulate is still that system's *pipeline*, not its architecture.

A "runner_count" of 2 to 4 is sensible. Use 1 when the point is precisely that
"parallelism belongs to the fleet, not to the file you are editing".

# Reference timings

So the seconds stay consistent instead of being invented afresh each time:

- checkout: 5s
- install dependencies (npm ci, go mod download, pip install): 40-90s
- lint / vet / format check: 15-30s
- unit tests: 60-150s
- build / compile: 40-120s
- build a docker image: 120-240s
- integration tests with a database: 150-300s
- browser e2e: 180-300s
- deploy / apply: 60-200s

# Hard constraints

- Every step used in examples must exist in the steps list.
- "consumes" must point at an artifact some other step "produces".
- Examples must be valid YAML, indented with spaces, never tabs.

# Follow-up turns: edit, do not rebuild

When the user sends another sentence they are **editing** the scenario you just
produced. Return the complete scenario, but **keep everything they did not ask
you to change** — the same step names, the same seconds, the same examples.
"Change runners to 4" means changing exactly one number, not building a
different step catalog.

When you receive an error message from the engine, fix the thing it names and
leave the rest alone.

# Off-topic requests

If the question is not about a CI/CD pipeline, build the nearest step catalog
that still makes sense for the technology mentioned, and say plainly in the notes
field how you interpreted it.

# One complete scenario, in exactly the shape you must return

` + workedExample + `

Answer in English in notes, note and title.`
