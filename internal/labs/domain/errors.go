package domain

import "errors"

var (
	ErrLabNotFound    = errors.New("lab not found")
	ErrNotFound       = errors.New("session not found")
	ErrAlreadyRunning = errors.New("a session is already running")
	ErrNotRunning     = errors.New("session is not running")
	// Handing in with tasks still unpassed.
	ErrIncomplete = errors.New("lab is not finished")
	// Asking for the report of a session still in progress. The report carries
	// the answer key, so it is only assembled once the attempt is over.
	ErrStillRunning = errors.New("session is still running")
	ErrTaskNotFound = errors.New("task not found")
	// A task that exists but belongs to another lab. Kept apart from "not found"
	// because it is a client sending the wrong id, not a missing row.
	ErrTaskNotInLab = errors.New("task does not belong to this lab")
	// The check script ran past its deadline. Reported separately so the student
	// is told to retry rather than shown a server error for an author's bug.
	ErrCheckTimeout = errors.New("check script timed out")
	// Trying an empty script would always pass and teach the author nothing.
	ErrEmptyScript = errors.New("check script is empty")
	// The lab has no image pinned, so there is nothing to start a trial in.
	ErrNoImage = errors.New("lab has no image")
	// Grading a sim task before the student has run anything. There is no result
	// to read, and marking it failed would count an attempt they never made.
	ErrNoSimRun = errors.New("no simulated run yet")
	// The per-session run budget is spent. A bound on an author's runaway example
	// and on a client resending in a loop, not on an attacker: the route is behind
	// the student's own live session.
	ErrTooManyRuns = errors.New("too many simulated runs")
	// Asking a container lab to simulate a pipeline. A client error rather than a
	// missing row, which is why it is not ErrLabNotFound.
	ErrNotSimLab = errors.New("lab is not a simulation")
	// Two presses of Run landed on the same run number. Reported rather than
	// retried: a retry would store a run the student did not ask for.
	ErrRunRaced = errors.New("simulated run raced another")
	// A scenario the caller handed in with nothing usable in it. The sentence is in
	// Vietnamese because it is shown to the author with the specific reason
	// appended, the same way the pipeline parser's rejections are.
	ErrInvalidScenario = errors.New("kịch bản không dùng được")
	// Starting a lab of a course the student never signed up for. A container is
	// a real resource with a real cost, and enrolment is the record that says who
	// asked for this material — so the check belongs on the way in, not on the
	// button that happens to be the usual way there.
	ErrNotEnrolled = errors.New("not enrolled in the course")
	// The lab has no usable incident scenario. Not an error on its own — every
	// container lab answers this — so it is what tells the two kinds of lab
	// apart, in the one place that asks.
	ErrNoIncident = errors.New("lab has no incident scenario")
	// The daily budget for AI-generated scenarios is spent. Every generation is
	// a paid call against a real account, so this is a cost control before it is
	// anything else — and it is reported to the person rather than logged,
	// because a button that silently stops working reads as a broken feature.
	ErrAIQuota = errors.New("đã dùng hết lượt nhờ AI dựng kịch bản hôm nay")
)
