package domain

import "time"

// Stats is the admin overview in one read. Deliberately a snapshot of totals
// rather than a series: the question the screen answers is "what does the
// platform look like right now", and a chart of it is a different screen.
type Stats struct {
	// Everyone with an account, and the ones who actually opened a lab in the
	// last week. The gap between the two is the number that means something —
	// registrations on their own say nothing about use.
	Students       int
	ActiveStudents int
	Courses        int
	Published      int
	Sessions       int
	SessionsWeek   int
	// Sessions with a container alive right now. This is the only figure here
	// that costs money while nobody is looking at it.
	Running   int
	Submitted int
	Labs      []LabStat
}

// RunningSession is one live container as the admin screen lists it. Carries
// who and what rather than ids alone: the decision an admin makes here is
// "should this person still have this running", and an id answers neither half.
type RunningSession struct {
	ID string
	// Carried so the screen can link to what this person has been doing. The
	// decision here is about a person, and a name with nothing behind it makes
	// the admin go and search for them by hand.
	UserID    int64
	Username  string
	LabTitle  string
	StartedAt time.Time
	ExpiresAt time.Time
	// Empty when the row was created but the container never came up. Worth
	// showing: it is the shape an orphaned session takes.
	HasContainer bool
}

// LabStat is one lab's traffic. Retried is what the attempt count bought: a lab
// where most answers took several presses is a lab whose questions are unclear
// or whose hints are missing, and nothing else on this screen can say that.
type LabStat struct {
	LabID       int64
	LabTitle    string
	CourseTitle string
	Sessions    int
	Submitted   int
	// Answers with an attempt count on record. Answers written before the count
	// existed are left out of both this and Retried rather than assumed to be
	// first-try passes.
	Answered int
	Retried  int
}
