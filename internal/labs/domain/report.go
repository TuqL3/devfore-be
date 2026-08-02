package domain

import "time"

// HistoryRow is one past attempt as the history list shows it: enough to
// recognise the lab and decide whether to open it, nothing more.
type HistoryRow struct {
	SessionID  string
	LabTitle   string
	LabSlug    string
	CourseSlug string
	Status     Status
	StartedAt  time.Time
	EndedAt    *time.Time
	// Of the lab's tasks, how many this session answered correctly.
	Correct int
	Total   int
}

// ReportAnswer is one question in a finished session, with the key alongside
// what the student picked. The key is only ever assembled for a session that has
// ended: while one is running, the same fields would be the answers.
type ReportAnswer struct {
	TaskID  int64
	Title   string
	Kind    string
	Points  int
	Options []string
	// Zero-based indexes into Options.
	Correct  []int
	Selected []int
	// Nil when the student never pressed check on this task.
	Passed     *bool
	AnsweredAt *time.Time
	// How many times check was pressed on this task. One means the answer on
	// record is the first one given. Nil for a session graded before the count
	// was kept, which is not the same as one and must not be shown as one.
	Attempts *int
}

// Report is one session, end to end: the header the result screen shows and the
// per-question detail under it.
type Report struct {
	SessionID   string
	LabTitle    string
	LabSlug     string
	CourseSlug  string
	Status      Status
	StartedAt   time.Time
	EndedAt     *time.Time
	SubmittedAt *time.Time
	Correct     int
	Total       int
	Answers     []ReportAnswer
}
