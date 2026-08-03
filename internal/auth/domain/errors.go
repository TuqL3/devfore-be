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

	// An admin aiming a moderation action at their own account. Refused rather
	// than allowed-with-a-warning: it is the one target where a mistake locks
	// the person making it out of the screen they would fix it from. Refusing
	// it is also what keeps at least one active admin on the system, since the
	// caller is one by definition and cannot remove themselves.
	ErrSelfTarget = errors.New("cannot moderate your own account")
	// Banning something that is not an active account. A pending signup would
	// come back from an unban as active, which is a way around email
	// verification rather than a moderation decision.
	ErrNotActive = errors.New("only an active account can be banned")

	// Starting enrolment on an account that already has a confirmed factor.
	// Refused rather than allowed to overwrite: anyone holding a live session
	// could otherwise move the second factor to a phone of their own.
	ErrTOTPEnabled = errors.New("two-factor is already on")
	// The account has no confirmed second factor, so there is nothing to
	// confirm, disable or answer a challenge with.
	ErrTOTPNotEnabled = errors.New("two-factor is not on")
)
