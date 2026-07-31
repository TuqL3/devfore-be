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
)

type Task struct {
	ID          int64
	LabID       int64
	CourseID    int64
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
}

// Grade is what a single press of the check button changed. PointsAwarded is
// zero for a task already passed, which is how a retry stays free.
type Grade struct {
	Passed        bool
	PointsAwarded int
	LabCompleted  bool
}
