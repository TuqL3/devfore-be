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

// SharedDrill is a finished drill as a stranger sees it — the numbers, the name
// of the fault, and nothing else.
//
// What is missing is the point. No timeline: it is a verbatim record of what
// somebody typed into a shell, which is where a mistyped password or an internal
// hostname ends up. No RevealMD: that is the walkthrough, and the same page
// offers the reader a go at this exact scenario. No email, no session id, no
// user id.
//
// IncidentTitle names the fault, so it is a spoiler for anyone about to try it.
// It travels anyway — the owner published a result and the result is about that
// fault — and the page keeps it behind a deliberate click.
type SharedDrill struct {
	Token    string
	LabSlug  string
	LabTitle string
	// Which scenario to hand the reader if they take the challenge. This is the
	// whole of "the same seed": a drill has no random number to replay, it has a
	// row that says how the service was broken.
	IncidentID    int64
	IncidentTitle string
	// Display name of whoever ran it. Never their email.
	Player    string
	StartedAt time.Time
	// False when the drill ended with the service still down, which is a result
	// rather than a missing number.
	Recovered       bool
	DowntimeSeconds int
	RequestsFailed  int
	RPS             int
}

// DrillScenario is one playable fault, as the daily pick sees the field: a lab
// paired with one of its scenarios. Labs carrying several appear several times,
// which is the intended weighting — a lab with four faults has four days' worth
// of material in it.
type DrillScenario struct {
	LabID      int64
	LabSlug    string
	LabTitle   string
	IncidentID int64
	// Carried so the cost of an outage is worked out in one place — every row of
	// the day's board is against this one scenario, so its rate is the rate.
	RPS int
}

// DrillLeader is one row of the daily board. Ranked by downtime, so the fastest
// recovery is first; drills that never recovered are not on the board at all,
// because "did not fix it" has no time to rank.
type DrillLeader struct {
	Player          string
	DowntimeSeconds int
	RequestsFailed  int
}

// DailyDrill is the scenario everybody gets today, and how everybody did on it.
//
// One scenario for the whole day, chosen from the date rather than at random, so
// two people comparing times are comparing the same fault. Day is the date the
// choice was made from, in UTC — a board that rolled over at each viewer's local
// midnight would be several boards.
type DailyDrill struct {
	Day        string
	LabSlug    string
	LabTitle   string
	IncidentID int64
	Leaders    []DrillLeader
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
