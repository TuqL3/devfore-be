package domain

import "time"

type Status string

const (
	StatusRunning Status = "running"
	StatusEnded   Status = "ended"
	StatusExpired Status = "expired"
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
}

// Spec is everything needed to build a container, read off the lab and the
// lab_images row it points at. duration_minutes is not in here: that is how long
// the lab is expected to take a person, not how long the container may live.
type Spec struct {
	LabID    int64
	LabSlug  string
	LabTitle string
	Image    string
}
