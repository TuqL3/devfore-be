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
	// Admin listings show drafts; the public one must not, which is the only
	// difference between the two queries.
	IncludeDrafts bool
}

// CourseInput is what an admin form sends. Separate from Course because the
// counts, timestamps and id on that one are computed, and letting a request
// carry them would mean deciding which ones to ignore on every write.
type CourseInput struct {
	Slug        string
	Title       string
	Description string
	ImageURL    *string
	Level       string
	Status      string
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
	// Which container image the lab starts. Null until an author picks one, and
	// a lab without it cannot be started at all — the session query joins images
	// inner. Only the admin presenter exposes it.
	LabImageID *int64
	// Filled by a second query, never by a scan. Without the tag gorm reads the
	// slice as a relation and fails every query that scans a Lab, including the
	// course detail one that does not want tasks at all.
	Tasks []Task `gorm:"-"`
}

// Task kinds. A script task is graded by running a shell command in the
// student's container; a choice task by comparing what they ticked.
const (
	KindScript = "script"
	KindChoice = "choice"
	// Graded from the shell history: the question asks the student to run a
	// command that changes nothing, so nothing else records that they did.
	KindCommand = "command"
)

// Task deliberately has no CheckScript field. The column holds the commands that
// decide whether a submission passes, so anything that reaches a presenter must
// not be able to carry it out to the client.
type Task struct {
	ID       int64
	Title    string
	Hint     string
	Points   int
	OrderIdx int
	Kind     string
	// Choices as the student sees them: the text only. Which ones are correct is
	// the answer key and lives in AdminTask.
	Options []string
	// Exactly one option is correct, so the question can be asked with radios.
	// It says how many, never which — the answer key stays in AdminTask.
	SingleAnswer bool
}

// Option is one answer of a choice question. Correct never leaves the admin API.
type Option struct {
	Text    string `json:"text"`
	Correct bool   `json:"correct"`
}

// AdminTask is Task plus the answer key. It exists as its own type so that
// adding a field here can never widen what the student-facing Task carries: the
// presenter for that one has a test asserting the script never reaches it.
type AdminTask struct {
	ID          int64
	Title       string
	Hint        string
	Points      int
	OrderIdx    int
	Kind        string
	CheckScript string
	Options     []Option
	// Accepted commands, one per line. The answer key for a command task, so it
	// never reaches the student-facing Task.
	ExpectedCommands string
}

// LabImage is the pinned container image an author picks for a lab.
type LabImage struct {
	ID          int64
	Name        string
	Tag         string
	Description string
	Active      bool
}

type LabInput struct {
	Slug            string
	Title           string
	DescriptionMD   string
	DurationMinutes int
	LabImageID      *int64
	OrderIdx        int
}

type TaskInput struct {
	Title            string
	Hint             string
	Kind             string
	CheckScript      string
	Options          []Option
	ExpectedCommands string
	Points           int
	OrderIdx         int
}

type Review struct {
	ID        int64
	Title     string
	ContentMD string
	OrderIdx  int
}

// ReviewInput is what an author submits for a revision note. Same shape minus
// the id, which the database owns.
type ReviewInput struct {
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

// Enrollment is one row of "my courses": the course, plus how far this student
// has got in it. The progress numbers come from course_scores, which the grading
// path already maintains — recomputing them here would be a second answer to the
// same question, free to disagree with the leaderboard.
type Enrollment struct {
	Course        Course
	Score         int
	LabsCompleted int
	EnrolledAt    time.Time
}
