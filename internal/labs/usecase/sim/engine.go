package sim

import (
	"fmt"
	"hash/fnv"
	"sort"

	"github.com/devforge/be/internal/labs/domain"
)

// Run schedules a parsed pipeline against a scenario and reports what happened.
//
// Pure by construction: seed and runIndex stand in for every source of variation
// a real pipeline has, and nothing else is consulted. Two calls with the same
// arguments return the same result, today and after a restart — which is the
// property that lets a run be stored once, graded, and graded again later.
//
// warm is the cache keys left populated by earlier runs of the same session.
func Run(
	sc *domain.Scenario, p *domain.Pipeline, seed string, runIndex int, warm []string,
) *domain.RunResult {
	e := &engine{
		sc:       sc,
		pipe:     p,
		seed:     seed,
		runIndex: runIndex,
		warm:     make(map[string]bool, len(warm)),
		produced: map[string]map[string]bool{},
		finished: map[string]*domain.RunJob{},
	}
	for _, k := range warm {
		e.warm[k] = true
	}

	names := JobNames(p)
	e.schedule(names)

	res := &domain.RunResult{
		RunIndex: runIndex,
		Status:   domain.RunSuccess,
		Jobs:     make([]domain.RunJob, 0, len(names)),
	}
	for _, name := range names {
		job := e.finished[name]
		if job == nil {
			// The scheduler places every job or skips it, so reaching here is a bug
			// in this file. Reported as a job rather than dropped: a pipeline whose
			// output silently omits a job the student wrote is the one failure mode
			// nobody would think to look for.
			job = &domain.RunJob{
				Name: name, Runner: -1, Status: domain.JobSkipped,
				Reason: "never scheduled", Steps: []domain.RunStep{},
			}
		}
		if job.Status != domain.JobSuccess {
			res.Status = domain.RunFailed
		}
		if job.End > res.TotalSeconds {
			res.TotalSeconds = job.End
		}
		res.Jobs = append(res.Jobs, *job)
	}
	res.CriticalPath = e.criticalPath()
	res.WarmCaches = e.warmAfter()
	res.Insights = insights(sc, p, res)
	return res
}

type engine struct {
	sc       *domain.Scenario
	pipe     *domain.Pipeline
	seed     string
	runIndex int
	warm     map[string]bool
	// What each finished job left behind. Only successful jobs get an entry: a job
	// that fell over halfway did not upload what it had not built yet.
	produced map[string]map[string]bool
	finished map[string]*domain.RunJob
}

// schedule walks the clock forward, placing every job that is ready onto a free
// runner. It jumps from one job ending to the next rather than ticking a second
// at a time: the only moments anything can change are the moments a job finishes.
func (e *engine) schedule(names []string) {
	runners := e.sc.RunnerCount
	if runners < 1 {
		runners = 1
	}
	// When each runner next comes free. Zero means free from the start.
	busy := make([]int, runners)

	pending := make(map[string]bool, len(names))
	for _, name := range names {
		pending[name] = true
	}

	clock := 0
	// Bounded rather than looping until pending empties. Each pass either places
	// work or advances the clock past a job's end, so the bound is generous; a
	// scheduler that cannot place a job should end the run, not hang the request.
	for guard := 0; len(pending) > 0 && guard <= len(names)+2; guard++ {
		for progress := true; progress; {
			progress = false
			for _, name := range names {
				if !pending[name] || !e.needsSettled(name, clock) {
					continue
				}
				if blocker := e.blocker(name); blocker != "" {
					e.finished[name] = skippedJob(name, clock, blocker)
					delete(pending, name)
					progress = true
					continue
				}
				runner := freeRunner(busy, clock)
				if runner < 0 {
					continue
				}
				job := e.execute(name, clock, runner)
				busy[runner] = job.End
				e.finished[name] = &job
				delete(pending, name)
				progress = true
			}
		}
		if len(pending) == 0 {
			return
		}
		next := nextRelease(busy, clock)
		if next <= clock {
			// Nothing running, nothing placeable: the graph would have to hold a
			// cycle, which Parse rejects. Leave the rest unscheduled and let Run
			// report them rather than spin.
			return
		}
		clock = next
	}
}

// needsSettled reports whether everything this job waits on has actually finished
// by now. A job is written into finished the moment it is placed, with an end
// time in the future, so membership alone is not enough — reading it that way
// would let a dependent start while the job it needs is still running.
func (e *engine) needsSettled(name string, clock int) bool {
	for _, need := range e.pipe.Jobs[name].Needs {
		job, ok := e.finished[need]
		if !ok || job.End > clock {
			return false
		}
	}
	return true
}

// blocker names the first dependency that did not succeed, in the order the
// student listed them, or "" when they all did. Their order rather than sorted
// order: the sentence is about the file they are looking at.
func (e *engine) blocker(name string) string {
	for _, need := range e.pipe.Jobs[name].Needs {
		if job, ok := e.finished[need]; ok && job.Status != domain.JobSuccess {
			return need
		}
	}
	return ""
}

// execute runs one job's steps back to back on a runner it already holds, and
// stops at the first step that fails: the steps after it never ran, so they are
// not in the result at all rather than in it marked skipped.
func (e *engine) execute(name string, start, runner int) domain.RunJob {
	spec := e.pipe.Jobs[name]
	job := domain.RunJob{
		Name: name, Runner: runner, Start: start, End: start,
		Status: domain.JobSuccess, Steps: make([]domain.RunStep, 0, len(spec.Steps)),
	}

	available := e.artifactsFor(name)
	asked := make(map[string]bool, len(spec.Cache))
	for _, key := range spec.Cache {
		asked[key] = true
	}
	produced := map[string]bool{}

	clock := start
	for i, uses := range spec.Steps {
		// Parse guarantees the key is in the catalogue, so a miss here is a step
		// that was validated against a different scenario than the one running.
		st := e.sc.Catalog[uses]
		step := domain.RunStep{
			Uses: uses, Start: clock, End: clock,
			Status: domain.JobSuccess, Flaky: st.Flaky > 0,
		}

		// Either an earlier step of this same job left it — one job is one
		// workspace, and `npm run build` followed by `docker build` is the most
		// ordinary shape a pipeline has — or a job in this one's needs chain did.
		// Anything else is not on this runner's disk.
		if st.Consumes != "" && !available[st.Consumes] && !produced[st.Consumes] {
			step.Status = domain.JobFailed
			step.Reason = fmt.Sprintf(
				"cần %q, mà không job nào nó phụ thuộc — và không step nào trước đó trong job này — tạo ra",
				st.Consumes)
			job.Steps = append(job.Steps, step)
			return failed(job, step, clock)
		}

		seconds := st.Seconds
		if st.Cacheable != "" && asked[st.Cacheable] && e.warm[st.Cacheable] {
			seconds = e.sc.CacheRestoreSeconds
			step.Cached = true
			step.CacheKey = st.Cacheable
		}
		// A step that fails still costs its time. A test suite that goes red has
		// run; billing the pipeline nothing for it would teach that failing is free.
		clock += seconds
		step.End = clock

		if st.Flaky > 0 && e.flakes(name, i, uses, st.Flaky) {
			step.Status = domain.JobFailed
			step.Reason = fmt.Sprintf("failed; this step fails %d%% of the time", st.Flaky)
			job.Steps = append(job.Steps, step)
			return failed(job, step, clock)
		}
		if st.Produces != "" {
			produced[st.Produces] = true
		}
		job.Steps = append(job.Steps, step)
	}

	job.End = clock
	e.produced[name] = produced
	return job
}

// artifactsFor is everything reachable through this job's needs, transitively.
// Reachability is the whole point: a job that happens to run earlier somewhere
// else in the graph leaves nothing on this runner's disk, and a pipeline that
// works by accident of scheduling is the bug this rule exists to surface.
func (e *engine) artifactsFor(name string) map[string]bool {
	available := map[string]bool{}
	seen := map[string]bool{name: true}
	queue := append([]string{}, e.pipe.Jobs[name].Needs...)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		for artifact := range e.produced[current] {
			available[artifact] = true
		}
		queue = append(queue, e.pipe.Jobs[current].Needs...)
	}
	return available
}

// flakes decides a flaky step's fate from a hash instead of a random source. The
// run number is part of the key, so pressing Run again is a genuinely different
// roll while re-grading the run already stored is not — a retry can pass, a
// replay cannot change its mind.
func (e *engine) flakes(job string, index int, uses string, percent int) bool {
	h := fnv.New32a()
	fmt.Fprintf(h, "%s|%d|%s|%d|%s", e.seed, e.runIndex, job, index, uses)
	return int(h.Sum32()%100) < percent
}

// criticalPath is the chain ending at the last job to finish, walked back through
// needs by whichever dependency finished latest. Ties break on sorted job order,
// so the answer is stable rather than merely usually the same.
func (e *engine) criticalPath() []string {
	last := ""
	for _, name := range JobNames(e.pipe) {
		job := e.finished[name]
		if job == nil || job.Runner < 0 {
			continue
		}
		if last == "" || job.End > e.finished[last].End {
			last = name
		}
	}
	if last == "" {
		return []string{}
	}

	chain := []string{last}
	for {
		previous := ""
		for _, need := range e.pipe.Jobs[last].Needs {
			job := e.finished[need]
			if job == nil || job.Runner < 0 {
				continue
			}
			if previous == "" || job.End > e.finished[previous].End {
				previous = need
			}
		}
		if previous == "" {
			return chain
		}
		chain = append([]string{previous}, chain...)
		last = previous
	}
}

// warmAfter is the cache keys the next run may restore: whatever was already warm
// plus whatever a job that asked for a key finished building.
//
// Across runs only, never within one. Real CI does save a cache mid-run and a
// later job in the same run can hit it — modelling that would make a job's
// duration depend on which runner happened to be free first, which is a lesson
// about scheduling smuggled into a lesson about caching. Deliberate v1 ceiling.
func (e *engine) warmAfter() []string {
	keys := make(map[string]bool, len(e.warm))
	for key := range e.warm {
		keys[key] = true
	}
	for _, name := range JobNames(e.pipe) {
		job := e.finished[name]
		if job == nil || job.Status != domain.JobSuccess {
			continue
		}
		spec := e.pipe.Jobs[name]
		asked := make(map[string]bool, len(spec.Cache))
		for _, key := range spec.Cache {
			asked[key] = true
		}
		for _, uses := range spec.Steps {
			if st := e.sc.Catalog[uses]; st.Cacheable != "" && asked[st.Cacheable] {
				keys[st.Cacheable] = true
			}
		}
	}

	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func failed(job domain.RunJob, step domain.RunStep, clock int) domain.RunJob {
	job.Status = domain.JobFailed
	job.Reason = step.Uses + " " + step.Reason
	job.End = clock
	return job
}

func skippedJob(name string, clock int, blocker string) *domain.RunJob {
	return &domain.RunJob{
		Name: name, Runner: -1, Start: clock, End: clock,
		Status: domain.JobSkipped,
		Reason: fmt.Sprintf("%s did not succeed", blocker),
		Steps:  []domain.RunStep{},
	}
}

// freeRunner returns the lowest-numbered runner free at clock, or -1. Lowest
// rather than any: the number ends up on screen as a lane, and a job hopping
// lanes between two runs of the same pipeline reads as a change that is not one.
func freeRunner(busy []int, clock int) int {
	for i, until := range busy {
		if until <= clock {
			return i
		}
	}
	return -1
}

// nextRelease is the first moment after clock at which a runner comes free.
func nextRelease(busy []int, clock int) int {
	next := 0
	for _, until := range busy {
		if until > clock && (next == 0 || until < next) {
			next = until
		}
	}
	return next
}
