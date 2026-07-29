package domain

import "errors"

var (
	ErrNotFound    = errors.New("course not found")
	ErrLabNotFound = errors.New("lab not found")
)
