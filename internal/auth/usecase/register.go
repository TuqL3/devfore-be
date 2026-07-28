package usecase

import (
	"context"
	"strings"

	"github.com/devforge/be/internal/auth/domain"
)

func (a *Auth) Register(ctx context.Context, in RegisterInput, meta SessionMeta) (AuthOutput, error) {
	hash, err := a.hasher.Hash(in.Password)
	if err != nil {
		return AuthOutput{}, err
	}
	u := &domain.User{
		Username:     in.Username,
		Email:        strings.ToLower(in.Email),
		PasswordHash: &hash,
		Status:       domain.StatusActive,
	}
	if err := a.users.Create(ctx, u, domain.RoleStudent); err != nil {
		return AuthOutput{}, err
	}
	u.Roles = []string{domain.RoleStudent}
	return a.start(ctx, u, meta)
}
