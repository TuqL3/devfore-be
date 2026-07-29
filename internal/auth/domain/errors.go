package domain

import "errors"

var (
	ErrNotFound     = errors.New("user not found")
	ErrConflict     = errors.New("email or username already in use")
	ErrCredentials  = errors.New("invalid credentials")
	ErrBanned       = errors.New("account banned")
	ErrInvalidToken = errors.New("invalid token")

	ErrNotVerified = errors.New("email not verified")
	ErrInvalidCode = errors.New("invalid or expired code")
	// Too many wrong codes for one address — the code is burned and a new one
	// has to be requested.
	ErrTooManyAttempts = errors.New("too many attempts")
	ErrRateLimited     = errors.New("try again later")
)
