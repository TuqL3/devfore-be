package domain

import "errors"

var (
	ErrLabNotFound    = errors.New("lab not found")
	ErrNotFound       = errors.New("session not found")
	ErrAlreadyRunning = errors.New("a session is already running")
	ErrNotRunning     = errors.New("session is not running")
)
