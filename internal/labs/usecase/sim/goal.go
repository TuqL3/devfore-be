package sim

import (
	"fmt"

	"github.com/devforge/be/internal/labs/domain"
)

// Eval decides whether a run meets a task's goal. Every clause must hold, and a
// goal with no clauses is an error rather than a pass: an empty goal would mark
// a student correct for a run that never left the starting line, and it is
// exactly what a half-finished task looks like in the database.
//
// The vocabulary is deliberately small. Each clause below is a fact about the
// schedule the engine produced, not about the text the student typed, so a lab
// asks for an outcome and accepts any pipeline that reaches it. Adding a clause
// type is a few lines here; adding an expression language would be a second
// grading system to get wrong.
func Eval(goal *domain.Goal, r *domain.RunResult) (bool, error) {
	if goal == nil || len(goal.All) == 0 {
		return false, domain.ErrEmptyGoal
	}
	for i, p := range goal.All {
		ok, err := evalOne(p, r)
		if err != nil {
			return false, fmt.Errorf("clause %d: %w", i+1, err)
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func evalOne(p domain.Predicate, r *domain.RunResult) (bool, error) {
	set := 0

	if p.RunStatus != nil {
		set++
		if r.Status != *p.RunStatus {
			return false, nil
		}
	}
	if p.TotalSecondsLTE != nil {
		set++
		if r.TotalSeconds > *p.TotalSecondsLTE {
			return false, nil
		}
	}
	if p.JobPresent != nil {
		set++
		if findJob(r, *p.JobPresent) == nil {
			return false, nil
		}
	}
	if p.JobStatus != nil {
		set++
		job := findJob(r, p.JobStatus.Job)
		// A job that is not in the pipeline at all fails the clause rather than
		// erroring: asking for "deploy is skipped" on a pipeline with no deploy
		// is a wrong answer, not a broken task.
		if job == nil || job.Status != p.JobStatus.Status {
			return false, nil
		}
	}
	if p.CacheHit != nil {
		set++
		if !cacheHit(r, *p.CacheHit) {
			return false, nil
		}
	}
	if len(p.JobsParallel) > 0 {
		set++
		if !allOverlap(r, p.JobsParallel) {
			return false, nil
		}
	}

	if set == 0 {
		// A clause that asks nothing is a typo in the goal — most likely a field
		// name the author guessed. Failing loudly beats quietly agreeing with it.
		return false, domain.ErrEmptyGoal
	}
	return true, nil
}

func findJob(r *domain.RunResult, name string) *domain.RunJob {
	for i := range r.Jobs {
		if r.Jobs[i].Name == name {
			return &r.Jobs[i]
		}
	}
	return nil
}

// cacheHit is true when the key was restored anywhere in the run. Any job, not a
// named one: what the lab is asking is whether the student stopped rebuilding the
// same thing, and which job got the benefit is their business.
func cacheHit(r *domain.RunResult, key string) bool {
	for _, job := range r.Jobs {
		for _, step := range job.Steps {
			if step.Cached && step.CacheKey == key {
				return true
			}
		}
	}
	return false
}

// allOverlap is true when every named job actually ran and every pair of them was
// alive at the same moment. Pairwise rather than "they share one instant": three
// jobs each overlapping the next by a second are not three jobs running together,
// and a lab asking for a fan-out means the fan-out.
//
// Touching at an endpoint does not count. One job ending as another begins is a
// handover on a single runner, which is the arrangement the clause exists to rule
// out.
func allOverlap(r *domain.RunResult, names []string) bool {
	jobs := make([]*domain.RunJob, 0, len(names))
	for _, name := range names {
		job := findJob(r, name)
		if job == nil || job.Runner < 0 || job.End <= job.Start {
			return false
		}
		jobs = append(jobs, job)
	}
	for i := 0; i < len(jobs); i++ {
		for j := i + 1; j < len(jobs); j++ {
			if jobs[i].Start >= jobs[j].End || jobs[j].Start >= jobs[i].End {
				return false
			}
		}
	}
	return true
}
