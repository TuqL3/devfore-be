package usecase

import (
	"context"

	"github.com/devforge/be/internal/auth/domain"
)

// Refresh trades a session id for a new access token — and a new session id.
//
// Rotating on every refresh is what makes a stolen refresh cookie a short
// problem instead of a permanent one: whichever side uses it first invalidates
// the copy the other side is holding.
func (a *Auth) Refresh(ctx context.Context, sessionID string, meta SessionMeta) (domain.Credentials, error) {
	old, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return domain.Credentials{}, domain.ErrInvalidToken
	}
	// Rotation mints a new id, but to the person reading the devices screen this
	// is still the same device signed in since the same moment. Carrying the
	// original timestamp forward is what keeps that true — otherwise every entry
	// would claim to have signed in minutes ago.
	meta.CreatedAt = old.CreatedAt

	u, err := a.users.ByID(ctx, old.UserID)
	if err != nil {
		// The account is gone. Drop the session instead of leaving it to be
		// retried forever against a user id that resolves to nothing.
		_ = a.sessions.Revoke(ctx, sessionID)
		return domain.Credentials{}, domain.ErrInvalidToken
	}
	if u.IsBanned() {
		// A ban has to reach the devices already signed in, not just new logins.
		_ = a.sessions.RevokeAll(ctx, u.ID, "")
		return domain.Credentials{}, domain.ErrBanned
	}

	out, err := a.start(ctx, u, meta)
	if err != nil {
		return domain.Credentials{}, err
	}
	// Revoked only once the replacement exists: failing here must not leave the
	// caller holding no session at all.
	if err := a.sessions.Revoke(ctx, sessionID); err != nil {
		return domain.Credentials{}, err
	}
	return out.Credentials, nil
}
