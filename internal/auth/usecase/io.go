package usecase

import "github.com/devforge/be/internal/auth/domain"

type RegisterInput struct {
	Username string
	Email    string
	Password string
}

type UpdateProfileInput struct {
	Username string
	Email    string
	// nil leaves the current avatar untouched; "" clears it.
	AvatarURL *string
}

type LoginInput struct {
	Login    string
	Password string
}

type AuthOutput struct {
	Tokens domain.TokenPair
	User   *domain.User
}

type GoogleProfile struct {
	ProviderID string
	Email      string
	Name       string
	AvatarURL  string
}
