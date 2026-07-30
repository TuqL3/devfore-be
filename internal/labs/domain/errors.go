package domain

import "errors"

var (
	ErrLabNotFound    = errors.New("lab not found")
	ErrNotFound       = errors.New("session not found")
	ErrAlreadyRunning = errors.New("a session is already running")
	ErrNotRunning     = errors.New("session is not running")
	ErrTaskNotFound   = errors.New("task not found")
	// A task that exists but belongs to another lab. Kept apart from "not found"
	// because it is a client sending the wrong id, not a missing row.
	ErrTaskNotInLab = errors.New("task does not belong to this lab")
	// The check script ran past its deadline. Reported separately so the student
	// is told to retry rather than shown a server error for an author's bug.
	ErrCheckTimeout = errors.New("check script timed out")
	// Trying an empty script would always pass and teach the author nothing.
	ErrEmptyScript = errors.New("check script is empty")
	// The lab has no image pinned, so there is nothing to start a trial in.
	ErrNoImage = errors.New("lab has no image")
)
