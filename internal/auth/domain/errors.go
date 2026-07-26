package domain

import "errors"

var (
	ErrNotFound     = errors.New("user not found")
	ErrConflict     = errors.New("email or username already in use")
	ErrCredentials  = errors.New("invalid credentials")
	ErrBanned       = errors.New("account banned")
	ErrInvalidToken = errors.New("invalid token")
)
