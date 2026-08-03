package usecase

import "github.com/devforge/be/internal/auth/domain"

type RegisterInput struct {
	Username string
	Email    string
	Password string
}

type UpdateProfileInput struct {
	Username  string
	Email     string
	AvatarURL *string
}

type LoginInput struct {
	Login    string
	Password string
}

// AuthOutput is either a finished login or a half-finished one. Exactly one of
// the two is filled: Challenge set means no session was created and the caller
// still owes a second factor, and reading Credentials in that case would hand
// out an empty token as if it were real.
type AuthOutput struct {
	Credentials domain.Credentials
	User        *domain.User
	// Non-nil when the account has a confirmed second factor.
	Challenge *domain.Challenge
}
