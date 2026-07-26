package rest

import (
	"time"

	"github.com/devforge/be/internal/auth/domain"
	"github.com/devforge/be/internal/auth/usecase"
)

type userResponse struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	AvatarURL *string   `json:"avatar_url"`
	Status    string    `json:"status"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
}

func newUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		AvatarURL: u.AvatarURL,
		Status:    string(u.Status),
		Roles:     u.Roles,
		CreatedAt: u.CreatedAt,
	}
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func newTokenResponse(tp domain.TokenPair) tokenResponse {
	return tokenResponse{
		AccessToken:  tp.AccessToken,
		RefreshToken: tp.RefreshToken,
		ExpiresIn:    tp.ExpiresIn,
	}
}

type authResponse struct {
	tokenResponse
	User userResponse `json:"user"`
}

func newAuthResponse(out usecase.AuthOutput) authResponse {
	return authResponse{
		tokenResponse: newTokenResponse(out.Tokens),
		User:          newUserResponse(out.User),
	}
}
