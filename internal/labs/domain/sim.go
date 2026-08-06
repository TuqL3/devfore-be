package domain

import (
	"errors"
	"time"
)

// The pipeline simulator's vocabulary. Three shapes: the Scenario an author
// writes once per lab, the Pipeline a student writes over and over, and the
// RunResult the engine produces from the two. Only the last is stored per press
// of Run, and it is the only one grading ever reads.

var (
	// A goal with no clauses would pass every run, including one that never
	// started. Refused when grading rather than when saving, so an author can keep
	// a half-written task the way an empty check script can be kept.
	ErrEmptyGoal = errors.New("sim task has no goal")
	// A scenario naming no steps leaves nothing a pipeline could be written
	// against, so there is no run to grade and no error worth showing a student.
	ErrEmptyScenario = errors.New("sim scenario has no catalog")
)

// Scenario is the authored half of a sim lab, stored in labs.sim_scenario.
type Scenario struct {
	Version int `json:"version"`
	// How many jobs may run at once. One runner is a legitimate scenario: it is
	// how a student is shown that parallelism belongs to the fleet, not to the
	// pipeline they are editing.
	RunnerCount int `json:"runner_count"`
	// The steps a pipeline is allowed to name. Naming anything else is an error
	// rather than a guess, and that is the line keeping this engine finite: steps
	// are picked from a catalogue, never interpreted as shell.
	Catalog map[string]ScenarioStep `json:"catalog"`
	// What a warm cache costs in place of the step's own seconds.
	CacheRestoreSeconds int `json:"cache_restore_seconds"`
	// Pipelines the author wrote to be loaded with one click. The engine never
	// reads them; they are here so that `make check-sim` runs each one and a
	// broken example is caught before somebody clicks it.
	//
	// They exist because the lesson is a comparison — the same work arranged two
	// ways takes two different times — and a comparison needs both sides. An
	// empty editor asks a newcomer to invent both.
	Examples []ScenarioExample `json:"examples,omitempty"`
}

type ScenarioExample struct {
	Title string `json:"title"`
	// One line on what this arrangement costs and why. Shown next to the button,
	// because "Nối tiếp" alone does not say what is about to be demonstrated.
	Note     string `json:"note,omitempty"`
	Pipeline string `json:"pipeline"`
}

// ScenarioStep is one catalogue entry: how long it takes and what it does to the
// things around it.
type ScenarioStep struct {
	Seconds int `json:"seconds"`
	// The cache key this step's work belongs to, empty when it leaves nothing
	// worth keeping. A job only gets the discount if it asks for the key by name,
	// which is the point: forgetting to declare the cache is the mistake being
	// taught, and it has to cost something.
	Cacheable string `json:"cacheable"`
	// What this step leaves behind for later jobs, empty when nothing.
	Produces string `json:"produces"`
	// What this step needs to have been left behind. It must come from a job this
	// one depends on: an artifact built by a job running somewhere else in the
	// graph is not something a real runner would find on disk.
	Consumes string `json:"consumes"`
	// Percent chance of failing, 0..100. Resolved from a seed rather than from a
	// random source, so the same pipeline on the same run number fails the same
	// way — while the next run number may not, which is what flakiness is.
	Flaky int `json:"flaky"`
}

// Pipeline is what the student wrote, parsed. The field names are the YAML ones:
// the file on screen is the thing being taught, so the struct does not get to
// rename its parts.
type Pipeline struct {
	Jobs map[string]PipelineJob `json:"jobs" yaml:"jobs"`
}

type PipelineJob struct {
	Needs []string `json:"needs,omitempty" yaml:"needs"`
	Steps []string `json:"steps"           yaml:"steps"`
	// Cache keys this job asks to reuse.
	Cache []string `json:"cache,omitempty" yaml:"cache"`
}

// Run and job outcomes. A run is only a success when every job is, so there is no
// third run-level value: a run with a skipped job did not do what was asked.
const (
	RunSuccess = "success"
	RunFailed  = "failed"

	JobSuccess = "success"
	JobFailed  = "failed"
	// Never started, because something it needed did not succeed. Kept apart from
	// failed: the job has no result of its own to read, and calling it failed
	// would point a student at the wrong job.
	JobSkipped = "skipped"
)

// RunResult is one press of Run, end to end. Stored as-is and replayed by the
// client, which is why it carries times rather than a stream of events: the
// schedule is already fully known the moment the engine returns.
type RunResult struct {
	RunIndex     int    `json:"run_index"`
	Status       string `json:"status"`
	TotalSeconds int    `json:"total_seconds"`
	// The chain of jobs ending at the last one to finish, walked back through
	// needs. An approximation when there are fewer runners than ready jobs:
	// queueing for a runner is not a dependency, so the chain does not show it.
	CriticalPath []string `json:"critical_path"`
	Jobs         []RunJob `json:"jobs"`
	Insights     []string `json:"insights"`
	// Cache keys warm after this run, carried into the next one. Kept with the
	// result rather than recomputed or sent by the client: the next run is graded
	// from this, and a value the client supplies is a value the client can lie
	// about.
	WarmCaches []string `json:"warm_caches"`
}

type RunJob struct {
	Name string `json:"name"`
	// Which runner picked it up, or -1 for a job that never ran.
	Runner int       `json:"runner"`
	Start  int       `json:"start"`
	End    int       `json:"end"`
	Status string    `json:"status"`
	Reason string    `json:"reason,omitempty"`
	Steps  []RunStep `json:"steps"`
}

type RunStep struct {
	Uses   string `json:"uses"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Status string `json:"status"`
	// Set when the work was restored instead of done. The key travels alongside so
	// a grader can ask about one specific cache without reading prose.
	Cached   bool   `json:"cached"`
	CacheKey string `json:"cache_key,omitempty"`
	// Set when this step is one the scenario declared flaky, whether or not it
	// failed this time. A flag rather than a phrase in Reason: everything that
	// reads a run back — grading, insights, the client — would otherwise be
	// matching on prose the engine wrote, and prose is the one field that changes.
	Flaky  bool   `json:"flaky,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// SimRun is one stored press of Run. The pipeline travels with the result
// because a result on its own cannot be checked afterwards: grading reads the
// stored result, and without the text that produced it there is no telling a
// faithful replay from a rewrite.
type SimRun struct {
	RunIndex  int        `json:"run_index"`
	Pipeline  string     `json:"pipeline"`
	Result    *RunResult `json:"result"`
	CreatedAt time.Time  `json:"created_at"`
}

// Goal is the pass condition of a sim task, stored in lab_tasks.sim_goal. Every
// clause must hold. There is deliberately no "any": a lab accepting either of two
// answers is two tasks, and an author who wants a boolean tree is describing a
// check script, which already exists.
type Goal struct {
	All []Predicate `json:"all"`
}

// Predicate is one clause. Pointers rather than zero values because a zero is a
// real answer here — total_seconds_lte of 0 is a cruel but meaningful
// requirement, and it must not read as "unset".
type Predicate struct {
	RunStatus       *string `json:"run_status,omitempty"`
	TotalSecondsLTE *int    `json:"total_seconds_lte,omitempty"`
	// Jobs whose run windows must overlap. States a fact about the schedule rather
	// than about the text, so a student may reach it however they like.
	JobsParallel []string `json:"jobs_parallel,omitempty"`
	// A cache key that was restored rather than rebuilt somewhere in the run.
	CacheHit *string `json:"cache_hit,omitempty"`
	// A job by that name exists in the pipeline, whatever became of it.
	JobPresent *string `json:"job_present,omitempty"`
	// What became of one named job. The only clause that can tell `skipped` from
	// `failed`, which is the whole of the fail-fast lesson: the job that broke and
	// the jobs that never got to try are two different things, and a lab teaching
	// that has to be able to ask for each.
	JobStatus *JobStatusClause `json:"job_status,omitempty"`
}

type JobStatusClause struct {
	Job string `json:"job"`
	// One of success, failed, skipped.
	Status string `json:"status"`
}

// AITurn is one message in a scenario-generation conversation. It lives in the
// domain rather than in either side because both the transport and the usecase
// name it, and neither should have to import the other to say "conversation".
//
// Assistant turns carry the JSON produced last time, which is what makes a
// follow-up like "đổi runner thành 4" mean something instead of starting over.
type AITurn struct {
	Assistant bool
	Text      string
}
