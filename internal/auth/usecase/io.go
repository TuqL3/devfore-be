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

type AuthOutput struct {
	Credentials domain.Credentials
	User        *domain.User
}

type GoogleProfile struct {
	ProviderID string
	Email      string
	Name       string
	AvatarURL  string
}
