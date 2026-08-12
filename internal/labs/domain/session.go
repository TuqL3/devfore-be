package domain

import "time"

type Status string

const (
	StatusRunning Status = "running"
	StatusEnded   Status = "ended"
	StatusExpired Status = "expired"
	// Handed in by the student. Ends the session like the two above, and is kept
	// apart from them because only this one means the answers were final.
	StatusSubmitted Status = "submitted"
)

type Session struct {
	ID          string
	UserID      int64
	LabID       int64
	ContainerID string
	Status      Status
	StartedAt   time.Time
	ExpiresAt   time.Time
	EndedAt     *time.Time
	// Where the session lives, so a client holding one can navigate back to it.
	// A lab id alone is only resolvable from the course the lab belongs to, which
	// is the one page a blocked student is not on.
	LabSlug    string
	CourseSlug string
	// The incident this session drew, nil for a normal lab. Kept on the session
	// rather than looked up from the lab, because the lab has several and only
	// the row knows which one this attempt is living through.
	IncidentID *int64
	// The drawn scenario's assumed request rate, read alongside the session so a
	// screen that shows the running cost does not cost a second query on every
	// poll. Zero whenever IncidentID is nil, and never the fault's name: while
	// the drill runs, the rate is the only part of the scenario that is not a
	// clue.
	IncidentRPS int
}

// Spec is everything needed to build a container, read off the lab and the
// lab_images row it points at. duration_minutes is not in here: that is how long
// the lab is expected to take a person, not how long the container may live.
type Spec struct {
	LabID      int64
	LabSlug    string
	LabTitle   string
	CourseSlug string
	Image      string
	// The authored scenario, raw from the jsonb column, empty for a lab that runs
	// in a container. Its presence is the whole of what makes a lab a sim lab:
	// there is no second flag that could disagree with it.
	SimScenario []byte
	// Builds the service an incident scenario then breaks. Empty on every lab
	// that is not an incident lab, and read together with the spec because it
	// runs in the same breath as the container being created.
	IncidentSetup string
	// Whether this lab has a fault ready to hand out. It decides three things at
	// once, which is why it is read here rather than asked for three times: the
	// container gets broken on the way up, no enrolment is required, and the
	// session's deadline comes from the drill instead of the global lab TTL.
	IsIncident bool
	// How long the drill gives you, in minutes. On an ordinary lab this is how
	// long the material is expected to take and nothing enforces it; on a drill
	// it is the deadline — the clock the whole challenge is built around.
	DurationMinutes int
}
