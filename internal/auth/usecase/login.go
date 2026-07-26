package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/devforge/be/internal/auth/domain"
)

func (a *Auth) Login(ctx context.Context, in LoginInput) (AuthOutput, error) {
	u, err := a.users.ByLogin(ctx, strings.ToLower(in.Login))
	if errors.Is(err, domain.ErrNotFound) {
		return AuthOutput{}, domain.ErrCredentials
	}
	if err != nil {
		return AuthOutput{}, err
	}
	if !u.HasPassword() || !a.hasher.Check(*u.PasswordHash, in.Password) {
		return AuthOutput{}, domain.ErrCredentials
	}
	if u.IsBanned() {
		return AuthOutput{}, domain.ErrBanned
	}
	tp, err := a.tokens.Issue(u.ID, u.Roles)
	return AuthOutput{Tokens: tp, User: u}, err
}
