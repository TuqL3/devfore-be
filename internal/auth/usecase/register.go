package usecase

import (
	"context"
	"strings"

	"github.com/devforge/be/internal/auth/domain"
)

func (a *Auth) Register(ctx context.Context, in RegisterInput) error {
	hash, err := a.hasher.Hash(in.Password)
	if err != nil {
		return err
	}
	u := &domain.User{
		Username:     in.Username,
		Email:        strings.ToLower(in.Email),
		PasswordHash: &hash,
		Status:       domain.StatusPending,
	}
	if err := a.users.Create(ctx, u, domain.RoleStudent); err != nil {
		return err
	}

	if err := a.sendCode(ctx, u.Email); err != nil {
		_ = a.users.Delete(ctx, u.ID)
		return err
	}
	return nil
}
