package usecase

import (
	"context"
	"strings"

	"github.com/devforge/be/internal/auth/domain"
)

// Register no longer signs anyone in: the account is held as pending and the
// caller has to come back through VerifyEmail with the code. Delivering that
// code is the only thing that proves the address is real.
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
		// Keeping the row would be worse than losing it: the username and email
		// would be held by an account nobody can verify, and the retry everyone
		// reaches for would come back as a conflict.
		_ = a.users.Delete(ctx, u.ID)
		return err
	}
	return nil
}
