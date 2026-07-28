package usecase

import (
	"context"

	"github.com/devforge/be/internal/auth/domain"
)

func (a *Auth) Refresh(ctx context.Context, sessionID string, meta SessionMeta) (domain.Credentials, error) {
	old, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return domain.Credentials{}, domain.ErrInvalidToken
	}
	meta.CreatedAt = old.CreatedAt

	u, err := a.users.ByID(ctx, old.UserID)
	if err != nil {
		_ = a.sessions.Revoke(ctx, sessionID)
		return domain.Credentials{}, domain.ErrInvalidToken
	}
	if u.IsBanned() {
		_ = a.sessions.RevokeAll(ctx, u.ID, "")
		return domain.Credentials{}, domain.ErrBanned
	}

	out, err := a.start(ctx, u, meta)
	if err != nil {
		return domain.Credentials{}, err
	}
	if err := a.sessions.Revoke(ctx, sessionID); err != nil {
		return domain.Credentials{}, err
	}
	return out.Credentials, nil
}
