package usecase

import (
	"context"

	"github.com/devforge/be/internal/auth/domain"
)

// ListUsers is the admin table. No scoping to the caller: the role check on the
// route is the whole of the authorisation, and every row is meant to be visible
// to whoever passed it.
func (a *Auth) ListUsers(ctx context.Context, f domain.UserFilter) (*domain.ManagedUsers, error) {
	return a.users.ListUsers(ctx, f)
}

// SetBanned locks an account or lets it back in.
//
// The ban itself is only half the job: an access token already issued carries
// its own expiry and would keep working until it ran out. Revoking the sessions
// is what makes the ban take effect now rather than in fifteen minutes.
func (a *Auth) SetBanned(ctx context.Context, actor, target int64, banned bool, reason string) error {
	if actor == target {
		return domain.ErrSelfTarget
	}
	if !banned {
		return a.users.Unban(ctx, target)
	}
	if err := a.users.Ban(ctx, target, actor, reason); err != nil {
		return err
	}
	// Best effort by design: the row already says banned, and the login path
	// reads the row. A redis blip must not report the ban as failed and invite
	// an admin to press it again.
	return a.sessions.RevokeAll(ctx, target, "")
}

// SetAdmin grants or takes away the admin role.
//
// Sessions go the same way as they do on a ban, and for a sharper reason: roles
// are baked into the access token at login, so an admin whose role is revoked
// keeps reaching admin screens until that token expires. Revoking forces the
// next request to get a token built from the roles as they are now.
//
// Refusing actor == target is what keeps at least one active admin on the
// system. The caller is an active admin by the time this runs — that is what the
// route's role check established — so a rule that stops them removing themselves
// leaves them standing, and no separate "last admin" count is needed.
func (a *Auth) SetAdmin(ctx context.Context, actor, target int64, admin bool) error {
	if actor == target {
		return domain.ErrSelfTarget
	}
	if err := a.users.SetRole(ctx, target, domain.RoleAdmin, admin); err != nil {
		return err
	}
	return a.sessions.RevokeAll(ctx, target, "")
}
