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
	// Filled by a second query, never by a scan. Without the tag gorm reads the
	// slice as a relation and fails every query that scans a Lab, including the
	// course detail one that does not want tasks at all.
	Tasks []Task `gorm:"-"`
}

// Task deliberately has no CheckScript field. The column holds the commands that
// decide whether a submission passes, so anything that reaches a presenter must
// not be able to carry it out to the client.
type Task struct {
	ID       int64
	Title    string
	Points   int
	OrderIdx int
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
