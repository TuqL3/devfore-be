package domain

import "time"

type Course struct {
	ID           int64
	Slug         string
	Title        string
	Description  string
	ImageURL     *string
	Level        string
	Status       string
	PublishedAt  *time.Time
	UpdatedAt    time.Time
	LabCount     int
	StudentCount int64
	Enrolled     bool
	Labs         []Lab
}

// Level is the difficulty tier a course belongs to. Rank drives ordering and
// the difficulty meter in the UI.
// CourseFilter carries the query params the list endpoint accepts.
type CourseFilter struct {
	Level string
	Query string
}

type Level struct {
	Slug        string
	Label       string
	Hint        string
	Rank        int
	CourseCount int64
}

type Lab struct {
	ID              int64
	Slug            string
	Title           string
	DescriptionMD   string
	DurationMinutes int
	OrderIdx        int
	TaskCount       int
	Points          int
}

type Review struct {
	ID        int64
	Title     string
	ContentMD string
	OrderIdx  int
}

type LeaderRow struct {
	Username      string
	AvatarURL     *string
	Score         int
	LabsCompleted int
	Attempts      int
	UpdatedAt     time.Time
}
