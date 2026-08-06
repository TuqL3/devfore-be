package sim

import (
	"fmt"
	"sort"
	"strings"

	"github.com/devforge/be/internal/labs/domain"
)

// How many observations to show. A pipeline with twenty jobs can be wrong in
// twenty ways at once, and a wall of advice is read as none of it.
const maxInsights = 6

// insights reads the finished run back and says what it noticed. Fixed rules, not
// a model: every line has to be something an author could have predicted and a
// student can act on, and a rule that fires when it should not is worse than one
// that stays quiet.
//
// The lines are in Vietnamese for the same reason the parser's rejections are:
// they are shown beside the pipeline the student is editing, and they are the
// lesson rather than a diagnostic.
//
// The order is the order the rules run in, and each rule walks jobs sorted by
// name, so the same run always produces the same list.
//
// Rules that say at most one thing come first, and the ones that walk every job
// come after: the cap below cuts from the end, and a pipeline with eight uncached
// jobs would otherwise spend the whole list on eight halves of the same sentence
// while the line naming where the time actually went never appears.
func insights(sc *domain.Scenario, p *domain.Pipeline, r *domain.RunResult) []string {
	out := []string{}
	out = append(out, flakyFailures(sc, r)...)
	out = append(out, criticalChain(p, r)...)
	out = append(out, runnerQueue(sc, p, r)...)
	out = append(out, idleRunners(sc, r)...)
	out = append(out, uncachedRepeats(sc, p)...)
	out = append(out, deadCacheKey(sc, p)...)
	out = append(out, needlessWaits(sc, p)...)

	if len(out) > maxInsights {
		hidden := len(out) - maxInsights
		out = append(out[:maxInsights:maxInsights],
			fmt.Sprintf("còn %d nhận xét nữa không hiện ở đây", hidden))
	}
	return out
}

// uncachedRepeats finds work being redone that the scenario says is cacheable.
// Reported per job rather than only when two jobs duplicate each other: a single
// job that rebuilds its dependencies every run is already paying the bill.
func uncachedRepeats(sc *domain.Scenario, p *domain.Pipeline) []string {
	var out []string
	for _, name := range JobNames(p) {
		spec := p.Jobs[name]
		asked := make(map[string]bool, len(spec.Cache))
		for _, key := range spec.Cache {
			asked[key] = true
		}
		reported := map[string]bool{}
		for _, uses := range spec.Steps {
			st := sc.Catalog[uses]
			if st.Cacheable == "" || asked[st.Cacheable] || reported[st.Cacheable] {
				continue
			}
			saved := st.Seconds - sc.CacheRestoreSeconds
			if saved <= 0 {
				continue
			}
			reported[st.Cacheable] = true
			out = append(out, fmt.Sprintf(
				"job %q chạy %s mà không khai cache %q — khai vào thì từ lượt sau tiết kiệm %ds",
				name, uses, st.Cacheable, saved))
		}
	}
	return out
}

// needlessWaits finds a needs line that buys nothing. The two jobs could have run
// side by side, and on a busy pipeline that line is most of the wall clock.
//
// Judged from the pipeline rather than from the run: whether one job uses
// another's output is a property of what was written, and stays true on the run
// where the dependency happened to fail anyway.
func needlessWaits(sc *domain.Scenario, p *domain.Pipeline) []string {
	var out []string
	for _, name := range JobNames(p) {
		wants := consumedBy(sc, p, name)
		if len(wants) == 0 && len(p.Jobs[name].Needs) == 0 {
			continue
		}
		for _, need := range p.Jobs[name].Needs {
			gives := producedBy(sc, p, need)
			used := false
			for artifact := range wants {
				if gives[artifact] {
					used = true
					break
				}
			}
			if !used {
				out = append(out, fmt.Sprintf(
					"job %q chờ %q nhưng không dùng gì job đó tạo ra — bỏ dòng needs này thì hai job chạy song song",
					name, need))
			}
		}
	}
	return out
}

// idleRunners reports a fleet the pipeline never used. Silent when only one job
// ran: a pipeline with a single job is not failing to use its runners.
func idleRunners(sc *domain.Scenario, r *domain.RunResult) []string {
	runners := sc.RunnerCount
	if runners < 2 {
		return nil
	}
	ran := 0
	for _, job := range r.Jobs {
		if job.Runner >= 0 {
			ran++
		}
	}
	if ran < 2 {
		return nil
	}
	if peak := peakConcurrency(r); peak < runners {
		return []string{fmt.Sprintf(
			"có %d runner nhưng nhiều nhất chỉ %d job chạy cùng lúc",
			runners, peak)}
	}
	return nil
}

// flakyFailures explains a red run that no change to the pipeline caused, and is
// careful about what it promises: the next run really may pass, and saying so is
// the point. A student who believes a retry is a fix has learned the wrong thing,
// and one who believes a retry is pointless has learned a different wrong thing.
func flakyFailures(sc *domain.Scenario, r *domain.RunResult) []string {
	var out []string
	for _, job := range r.Jobs {
		for _, step := range job.Steps {
			if !step.Flaky || step.Status != domain.JobFailed {
				continue
			}
			out = append(out, fmt.Sprintf(
				"%s hỏng ở job %q vì nó flaky (%d%% số lượt) — chạy lại có thể đậu mà không sửa gì",
				step.Uses, job.Name, sc.Catalog[step.Uses].Flaky))
		}
	}
	return out
}

// criticalChain names the jobs whose lengths add up to the clock, so a student
// asking which job to attack gets an answer instead of a total.
//
// Quiet in three cases, each because the sentence would otherwise be false. On a
// red run the total is the moment something broke rather than the cost of the
// work. When a job had to queue for a runner, the chain is not the whole story:
// finishing an off-chain job sooner frees a machine sooner and does move the
// total, which is what runnerQueue below is for. And when every job is already on
// the chain there is nothing being ruled out, which is the only part worth
// saying.
func criticalChain(p *domain.Pipeline, r *domain.RunResult) []string {
	if r.Status != domain.RunSuccess || len(r.CriticalPath) < 2 {
		return nil
	}
	if _, waited := worstQueue(p, r); waited > 0 {
		return nil
	}

	on := make(map[string]bool, len(r.CriticalPath))
	for _, name := range r.CriticalPath {
		on[name] = true
	}
	off := 0
	for _, job := range r.Jobs {
		if job.Runner >= 0 && !on[job.Name] {
			off++
		}
	}
	if off == 0 {
		return nil
	}
	return []string{fmt.Sprintf(
		"đường găng là %s — %ds của lượt này nằm ở chuỗi đó, %d job ngoài nó có nhanh lên cũng không đổi tổng",
		strings.Join(r.CriticalPath, " → "), r.TotalSeconds, off)}
}

// runnerQueue reports a job that was ready to go and had to wait for a machine.
// The distinction it draws is the one thing a pipeline cannot show on its own: a
// student who splits work into eight jobs and sees no improvement is reading the
// fleet size, not their own file, and nothing in the YAML says so.
//
// Only the longest wait is named. Behind a saturated fleet every later job
// queues, and eight lines saying that is one lesson repeated until it is skipped.
func runnerQueue(sc *domain.Scenario, p *domain.Pipeline, r *domain.RunResult) []string {
	name, waited := worstQueue(p, r)
	if name == "" {
		return nil
	}
	return []string{fmt.Sprintf(
		"job %q sẵn sàng chạy nhưng đợi %ds mới tới lượt: cả pipeline chỉ có %d runner — tách thêm job nữa không nhanh hơn",
		name, waited, sc.RunnerCount)}
}

// deadCacheKey finds a cache line that can never save anything, because no step
// in that job puts work under the key it names.
//
// Parse cannot catch this and should not try: a cache key is a name the author
// invents, so there is no list to check a spelling against. That leaves this the
// only place a student is ever told that the line they added does nothing — and a
// line that looks like it worked is worse than no line, because the next thing
// they conclude is that caching does not help.
func deadCacheKey(sc *domain.Scenario, p *domain.Pipeline) []string {
	var out []string
	for _, name := range JobNames(p) {
		spec := p.Jobs[name]
		stores := make(map[string]bool, len(spec.Steps))
		for _, uses := range spec.Steps {
			if key := sc.Catalog[uses].Cacheable; key != "" {
				stores[key] = true
			}
		}
		reported := map[string]bool{}
		for _, key := range spec.Cache {
			if stores[key] || reported[key] {
				continue
			}
			reported[key] = true
			out = append(out, fmt.Sprintf(
				"job %q khai cache %q nhưng không step nào trong job lưu gì dưới khoá đó — dòng cache này không tiết kiệm được gì",
				name, key))
		}
	}
	return out
}

// worstQueue is the job that waited longest between being allowed to start and
// actually starting, and how long it waited. Empty when nothing queued.
//
// A wait here can only be the fleet: the engine places a job the moment its needs
// have settled and a runner is free, so a gap after the needs settled is a gap
// with no free runner in it.
func worstQueue(p *domain.Pipeline, r *domain.RunResult) (string, int) {
	name, waited := "", 0
	// r.Jobs is in sorted name order, so a tie resolves the same way every time.
	for _, job := range r.Jobs {
		if job.Runner < 0 {
			continue
		}
		if delay := job.Start - readyAt(p, r, job.Name); delay > waited {
			name, waited = job.Name, delay
		}
	}
	return name, waited
}

// readyAt is the moment every job this one waits on had finished — the earliest
// it could legally have started. Zero for a job with no needs.
func readyAt(p *domain.Pipeline, r *domain.RunResult, name string) int {
	at := 0
	for _, need := range p.Jobs[name].Needs {
		if job := runJob(r, need); job != nil && job.End > at {
			at = job.End
		}
	}
	return at
}

func runJob(r *domain.RunResult, name string) *domain.RunJob {
	for i := range r.Jobs {
		if r.Jobs[i].Name == name {
			return &r.Jobs[i]
		}
	}
	return nil
}

// peakConcurrency is the largest number of jobs alive at one moment, counted by
// sweeping starts and ends. Ends are processed before starts at the same instant,
// so a job beginning exactly as another finishes is a handover, not an overlap —
// the same rule jobsParallel grades by.
func peakConcurrency(r *domain.RunResult) int {
	type event struct {
		at    int
		delta int
	}
	var events []event
	for _, job := range r.Jobs {
		if job.Runner < 0 || job.End <= job.Start {
			continue
		}
		events = append(events, event{job.Start, 1}, event{job.End, -1})
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].at != events[j].at {
			return events[i].at < events[j].at
		}
		return events[i].delta < events[j].delta
	})

	peak, live := 0, 0
	for _, e := range events {
		live += e.delta
		if live > peak {
			peak = live
		}
	}
	return peak
}

// consumedBy is the artifacts a job's own steps ask for.
func consumedBy(sc *domain.Scenario, p *domain.Pipeline, name string) map[string]bool {
	out := map[string]bool{}
	for _, uses := range p.Jobs[name].Steps {
		if st := sc.Catalog[uses]; st.Consumes != "" {
			out[st.Consumes] = true
		}
	}
	return out
}

// producedBy is everything a job and everything it depends on would leave behind.
// Transitive, because a needs line inherits the whole chain behind it.
func producedBy(sc *domain.Scenario, p *domain.Pipeline, name string) map[string]bool {
	out := map[string]bool{}
	seen := map[string]bool{}
	queue := []string{name}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		for _, uses := range p.Jobs[current].Steps {
			if st := sc.Catalog[uses]; st.Produces != "" {
				out[st.Produces] = true
			}
		}
		queue = append(queue, p.Jobs[current].Needs...)
	}
	return out
}
