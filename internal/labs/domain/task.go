package domain

// Task is the answer key side of a lab task: what to run and what passing it is
// worth. The student-facing half (title, hint) is served by the courses module,
// which reads the same table without this column.
// Task kinds, mirroring the courses module that owns the table. Two constants
// rather than an import: the modules do not depend on each other, and the value
// they agree on is a string in one column.
const (
	KindScript  = "script"
	KindChoice  = "choice"
	KindCommand = "command"
	// Graded against the student's most recent simulated pipeline run rather than
	// against a container. The only kind whose evidence was produced by this
	// server instead of read off a filesystem.
	KindSim = "sim"
)

type Task struct {
	ID    int64
	LabID int64
	// nil for a War Room challenge, which belongs to no course. Everything that
	// writes a course scoreboard has to check it — a drill earns a report, not
	// points on somebody's course.
	CourseID    *int64
	Points      int
	Kind        string
	CheckScript string
	// Correct answer indexes, in the order the student is shown the options.
	// Read on the server and never sent anywhere.
	CorrectOptions []int
	OptionCount    int
	// Accepted commands for a command task, one per line, already whitespace
	// normalised by the editor that saved them.
	ExpectedCommands string
	// The pass condition of a sim task, raw from the jsonb column. Carried
	// undecoded because the shape it decodes to belongs to the grader, and a task
	// of any other kind has `{}` here — a value worth moving around but not worth
	// parsing on every check.
	SimGoal []byte
}

// Grade is what a single press of the check button changed. PointsAwarded is
// zero for a task already passed, which is how a retry stays free.
type Grade struct {
	Passed        bool
	PointsAwarded int
	LabCompleted  bool
}
