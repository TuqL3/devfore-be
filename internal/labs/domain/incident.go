package domain

import "time"

// Incident is one authored fault: the state a lab's service is left in when the
// session starts. A lab carries several and one is drawn at random per session,
// which is what stops a second attempt being a recall exercise.
//
// The student is never told which one they drew. Every scenario of a lab breaks
// the same service, so what they see — the service is down — is identical, and
// only the cause differs.
type Incident struct {
	ID    int64
	Title string
	// The answer key. It runs inside the student's own container and its text
	// states the fault outright, so it is read on the server and goes nowhere
	// else while the attempt is live.
	BreakScript string
	// Read after the drill: what had broken, and how it is normally found.
	RevealMD string
	// Requests per second the outage is assumed to hurt. Authored, not measured:
	// it is what turns "six minutes" into "a few thousand people saw an error",
	// and whatever shows it has to say it is a simulated figure — the same
	// caveat the pipeline simulator carries on its seconds.
	RPS int
}

// TimelineEntry is one command the student ran during a drill, as the report
// replays it. What it teaches is not the command that fixed the service — it is
// the four minutes spent looking somewhere else first, which nothing but an
// ordered list of attempts can show.
type TimelineEntry struct {
	// Zero when the shell wrote the command without a timestamp, which is what a
	// history written before the rc file set one looks like. Rendered as "no
	// time" rather than as the epoch: a wrong time is worse than a missing one.
	At      time.Time
	Command string
}

// IncidentReport is the drill as the result screen tells it: how long the outage
// lasted, what it is assumed to have cost, what the fault actually was, and every
// attempt made along the way.
//
// Only ever built for a session that has ended. Title and RevealMD name the fault
// outright, so while the attempt is live they are the answer.
type IncidentReport struct {
	Title    string
	RevealMD string
	RPS      int
	// Nil when the service was never restored — running out of time is a real
	// outcome of a drill, and a report that quietly showed a recovery time for it
	// would be describing something that did not happen.
	RecoveredAt *time.Time
	// Seconds from the session starting to the recovery being declared. Zero
	// while RecoveredAt is nil rather than "so far": the drill is over, and the
	// outage did not end.
	DowntimeSeconds int
	RequestsFailed  int
	Timeline        []TimelineEntry
}
