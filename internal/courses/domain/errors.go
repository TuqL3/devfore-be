package domain

import "errors"

var (
	ErrNotFound    = errors.New("course not found")
	ErrLabNotFound = errors.New("lab not found")
	// The slug is the course's URL, so a duplicate is a conflict rather than a
	// bad field: the form is right, the name is taken.
	ErrSlugTaken = errors.New("course slug already exists")
	// Lab slugs are unique across the whole table, not per course, because the
	// lab session lookup addresses them on their own.
	ErrLabSlugTaken   = errors.New("lab slug already exists")
	ErrTaskNotFound   = errors.New("task not found")
	ErrReviewNotFound = errors.New("review not found")
	// War Room scenarios.
	ErrIncidentNotFound = errors.New("incident not found")
	// A scenario somebody has already played cannot be deleted: finished sessions
	// point at the row that broke them and their reports read it back. The
	// database refuses it; this is that refusal with a name an admin can act on.
	ErrIncidentPlayed = errors.New("incident has been played")
	// Publishing a challenge whose scenarios are all switched off would put it in
	// the list and then break nothing when a student opened it.
	ErrNoActiveIncident = errors.New("drill has no active incident")
)

// InvalidInput names the field a form should highlight. Kept as a type rather
// than a set of sentinel errors so a new rule does not need a new export.
type InvalidInput struct {
	Field   string
	Message string
}

func (e InvalidInput) Error() string { return e.Field + ": " + e.Message }
