package usecase

import (
	"context"

	"github.com/devforge/be/internal/auth/domain"
)

func (a *Auth) Refresh(ctx context.Context, refreshToken string) (domain.TokenPair, error) {
	id, err := a.tokens.ParseRefresh(refreshToken)
	if err != nil {
		return domain.TokenPair{}, domain.ErrInvalidToken
	}
	u, err := a.users.ByID(ctx, id)
	if err != nil {
		return domain.TokenPair{}, domain.ErrInvalidToken
	}
	if u.IsBanned() {
		return domain.TokenPair{}, domain.ErrBanned
	}
	return a.tokens.Issue(u.ID, u.Roles)
}
