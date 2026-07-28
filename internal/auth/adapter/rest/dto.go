package rest

import "github.com/devforge/be/internal/auth/usecase"

type registerRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32,alphanum"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

func (r registerRequest) toInput() usecase.RegisterInput {
	return usecase.RegisterInput{Username: r.Username, Email: r.Email, Password: r.Password}
}

type updateProfileRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32,alphanum"`
	Email    string `json:"email" binding:"required,email"`
	// Pointer so an omitted field keeps the current avatar while "" clears it.
	AvatarURL *string `json:"avatar_url" binding:"omitempty,max=500"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password" binding:"required,min=8,max=72"`
}

type deleteAccountRequest struct {
	Password string `json:"password"`
}

type loginRequest struct {
	Login    string `json:"login" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (r loginRequest) toInput() usecase.LoginInput {
	return usecase.LoginInput{Login: r.Login, Password: r.Password}
}
