package usecase

import (
	"context"
	"strings"

	"github.com/devforge/be/internal/auth/domain"
)

type Auth struct {
	users    UserRepository
	tokens   TokenIssuer
	hasher   PasswordHasher
	oauth    OAuthExchanger
	sessions SessionStore
}

func NewAuth(users UserRepository, tokens TokenIssuer, hasher PasswordHasher, oauth OAuthExchanger, sessions SessionStore) *Auth {
	return &Auth{users: users, tokens: tokens, hasher: hasher, oauth: oauth, sessions: sessions}
}

// start opens a session for a user who has just proved who they are. Every
// entry point — password login, registration, Google — funnels through here so
// there is exactly one place that decides what a fresh login is worth.
func (a *Auth) start(ctx context.Context, u *domain.User, meta SessionMeta) (AuthOutput, error) {
	// Session first: the access token embeds its id, so it cannot be signed
	// until the session it names actually exists.
	s, err := a.sessions.Create(ctx, u.ID, meta)
	if err != nil {
		return AuthOutput{}, err
	}
	access, ttl, err := a.tokens.IssueAccess(u.ID, u.Roles, s.ID)
	if err != nil {
		return AuthOutput{}, err
	}
	return AuthOutput{
		Credentials: domain.Credentials{
			AccessToken: access,
			AccessTTL:   ttl,
			SessionID:   s.ID,
			SessionTTL:  a.sessions.TTL(),
		},
		User: u,
	}, nil
}

// Sessions lists the devices signed in to an account, newest first.
func (a *Auth) Sessions(ctx context.Context, userID int64) ([]domain.Session, error) {
	return a.sessions.List(ctx, userID)
}

// RevokeSession signs out one specific device.
//
// The ownership check is the whole point: session ids are the only credential
// a session has, so accepting one from whoever asks would turn this endpoint
// into "sign out any user you can name an id for".
func (a *Auth) RevokeSession(ctx context.Context, userID int64, sessionID string) error {
	s, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return domain.ErrNotFound
	}
	if s.UserID != userID {
		// Deliberately the same error as "no such session": telling a caller
		// that an id exists but belongs to someone else is itself a leak.
		return domain.ErrNotFound
	}
	return a.sessions.Revoke(ctx, sessionID)
}

// Logout ends one session — this browser only. An unknown id is not an error:
// signing out twice, or with a cookie the server already dropped, should still
// leave the caller signed out.
func (a *Auth) Logout(ctx context.Context, sessionID string) error {
	return a.sessions.Revoke(ctx, sessionID)
}

// LogoutAll ends every session the account has anywhere. This is the answer to
// a lost laptop, and the reason refresh state lives in Redis at all.
func (a *Auth) LogoutAll(ctx context.Context, userID int64) error {
	return a.sessions.RevokeAll(ctx, userID, "")
}

func (a *Auth) Authorize(token string) (userID int64, roles []string, sessionID string, err error) {
	return a.tokens.ParseAccess(token)
}

func (a *Auth) Me(ctx context.Context, id int64) (*domain.User, error) {
	return a.users.ByID(ctx, id)
}

// ChangePassword doubles as "set a password" for Google-only accounts, which
// have no hash to check against yet.
func (a *Auth) SetAvatar(ctx context.Context, id int64, url string) (*domain.User, error) {
	if err := a.users.UpdateAvatar(ctx, id, url); err != nil {
		return nil, err
	}
	return a.users.ByID(ctx, id)
}

// keepSessionID is the session that asked for the change: every *other* device
// is signed out, because a password change is how someone evicts whoever they
// think is logged in as them. Signing out the caller too would only punish the
// person doing the right thing.
func (a *Auth) ChangePassword(ctx context.Context, id int64, current, next, keepSessionID string) error {
	u, err := a.users.ByID(ctx, id)
	if err != nil {
		return err
	}
	if u.HasPassword() && !a.hasher.Check(*u.PasswordHash, current) {
		return domain.ErrCredentials
	}
	hash, err := a.hasher.Hash(next)
	if err != nil {
		return err
	}
	if err := a.users.UpdatePassword(ctx, id, hash); err != nil {
		return err
	}
	return a.sessions.RevokeAll(ctx, id, keepSessionID)
}

// DeleteAccount is irreversible. Accounts with a password must re-enter it;
// a stolen access token alone should not be enough to wipe an account.
func (a *Auth) DeleteAccount(ctx context.Context, id int64, password string) error {
	u, err := a.users.ByID(ctx, id)
	if err != nil {
		return err
	}
	if u.HasPassword() && !a.hasher.Check(*u.PasswordHash, password) {
		return domain.ErrCredentials
	}
	if err := a.users.Delete(ctx, id); err != nil {
		return err
	}
	// The rows are gone; leaving sessions pointing at a dead user id would let
	// a refresh resurrect a login for an account that no longer exists.
	return a.sessions.RevokeAll(ctx, id, "")
}

func (a *Auth) UpdateProfile(ctx context.Context, id int64, in UpdateProfileInput) (*domain.User, error) {
	if err := a.users.UpdateProfile(ctx, id, in.Username, strings.ToLower(in.Email), in.AvatarURL); err != nil {
		return nil, err
	}
	return a.users.ByID(ctx, id)
}
