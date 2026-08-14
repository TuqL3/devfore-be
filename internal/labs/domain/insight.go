package domain

import "time"

// The shapes the admin screens read. All of them are answers to a question
// somebody would otherwise have to open psql for, and each one names the
// question in its comment rather than describing its columns.

// TaskHealth: is this task teaching, or is its check script broken?
//
// Both ends of the range are worth a look and they mean opposite things. Nobody
// passing usually means the script is wrong rather than the question being hard;
// everybody passing first time means the question is not asking anything.
type TaskHealth struct {
	TaskID    int64  `json:"task_id"`
	LabID     int64  `json:"lab_id"`
	LabSlug   string `json:"lab_slug"`
	LabTitle  string `json:"lab_title"`
	TaskTitle string `json:"task_title"`
	Kind      string `json:"kind"`
	Attempts  int    `json:"attempts"`
	Passed    int    `json:"passed"`
	PassRate  int    `json:"pass_rate"`
}

// LabHealth: do people finish this lab, or do they walk away from it?
type LabHealth struct {
	LabID     int64  `json:"lab_id"`
	LabSlug   string `json:"lab_slug"`
	LabTitle  string `json:"lab_title"`
	Starts    int    `json:"starts"`
	Submitted int    `json:"submitted"`
	Expired   int    `json:"expired"`
	Ended     int    `json:"ended"`
	Running   int    `json:"running"`
	// Percent of attempts that ended as anything but a hand-in.
	DropRate int `json:"drop_rate"`
}

// IncidentHealth: has anybody ever beaten this scenario?
//
// Zero solved out of many attempts is the row that matters: a break script that
// leaves the service unfixable is indistinguishable from a hard puzzle until
// somebody counts.
type IncidentHealth struct {
	IncidentID    int64  `json:"incident_id"`
	IncidentTitle string `json:"incident_title"`
	Active        bool   `json:"active"`
	LabSlug       string `json:"lab_slug"`
	Attempts      int    `json:"attempts"`
	Solved        int    `json:"solved"`
	BestSeconds   int    `json:"best_seconds"`
}

// CourseHealth: the funnel from signing up to finishing something.
type CourseHealth struct {
	CourseID    int64  `json:"course_id"`
	CourseSlug  string `json:"course_slug"`
	CourseTitle string `json:"course_title"`
	Status      string `json:"status"`
	Enrolled    int    `json:"enrolled"`
	Started     int    `json:"started"`
	Finished    int    `json:"finished"`
}

// SharedReport: one drill report that is public right now.
type SharedReport struct {
	Token         string    `json:"token"`
	SessionID     string    `json:"session_id"`
	Player        string    `json:"player"`
	LabTitle      string    `json:"lab_title"`
	IncidentTitle string    `json:"incident_title"`
	StartedAt     time.Time `json:"started_at"`
}

// UserSummary heads one person's activity page.
type UserSummary struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	Sessions     int       `json:"sessions"`
	Submitted    int       `json:"submitted"`
	SimRuns      int       `json:"sim_runs"`
	ChatMessages int       `json:"chat_messages"`
	Enrolments   int       `json:"enrolments"`
}

// UserSession is one attempt on that page.
type UserSession struct {
	SessionID     string     `json:"session_id"`
	LabSlug       string     `json:"lab_slug"`
	LabTitle      string     `json:"lab_title"`
	Status        string     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	EndedAt       *time.Time `json:"ended_at"`
	IncidentTitle string     `json:"incident_title"`
	Passed        int        `json:"passed"`
	Total         int        `json:"total"`
	// Whether there is a shell history to open. The history itself needs its own
	// request, because opening it is a thing worth recording.
	HasCommands bool `json:"has_commands"`
}

// UserSimRun is one pipeline somebody wrote and ran.
//
// Pipeline is what they typed. It is the most useful thing on the page — a
// wrong pipeline says exactly which part of the model has not landed yet — and
// it is truncated in the query, with the real length alongside so the screen can
// say so rather than pretending the text ended there.
type UserSimRun struct {
	ID             int64     `json:"id"`
	SessionID      string    `json:"session_id"`
	RunIndex       int       `json:"run_index"`
	CreatedAt      time.Time `json:"created_at"`
	Pipeline       string    `json:"pipeline"`
	PipelineLength int       `json:"pipeline_length"`
	LabTitle       string    `json:"lab_title"`
	TotalSeconds   int       `json:"total_seconds"`
}

// PlatformCounts is the strip across the top of the live screen. `since` scopes
// the ones that are about a window rather than about the whole history.
type PlatformCounts struct {
	Users         int `json:"users"`
	Banned        int `json:"banned"`
	NewUsers      int `json:"new_users"`
	Running       int `json:"running"`
	Sessions      int `json:"sessions"`
	Submitted     int `json:"submitted"`
	SimRuns       int `json:"sim_runs"`
	ChatMessages  int `json:"chat_messages"`
	SharedReports int `json:"shared_reports"`
}
