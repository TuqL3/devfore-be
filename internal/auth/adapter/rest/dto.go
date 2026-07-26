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

type loginRequest struct {
	Login    string `json:"login" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (r loginRequest) toInput() usecase.LoginInput {
	return usecase.LoginInput{Login: r.Login, Password: r.Password}
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}
