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
}
