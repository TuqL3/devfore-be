package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/devforge/be/internal/auth/domain"
)

type Verification struct {
	CodeTTL  time.Duration
	ResetTTL time.Duration
	Resend   time.Duration
	AppURL   string
}

type Auth struct {
	users    UserRepository
	tokens   TokenIssuer
	hasher   PasswordHasher
	oauth    OAuthExchanger
	sessions SessionStore
	codes    VerifyStore
	mailer   Mailer
	verify   Verification
}

func NewAuth(
	users UserRepository,
	tokens TokenIssuer,
	hasher PasswordHasher,
	oauth OAuthExchanger,
	sessions SessionStore,
	codes VerifyStore,
	mailer Mailer,
	verify Verification,
) *Auth {
	return &Auth{
		users:    users,
		tokens:   tokens,
		hasher:   hasher,
		oauth:    oauth,
		sessions: sessions,
		codes:    codes,
		mailer:   mailer,
		verify:   verify,
	}
}

func (a *Auth) start(ctx context.Context, u *domain.User, meta SessionMeta) (AuthOutput, error) {
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

func (a *Auth) Sessions(ctx context.Context, userID int64) ([]domain.Session, error) {
	return a.sessions.List(ctx, userID)
}

func (a *Auth) RevokeSession(ctx context.Context, userID int64, sessionID string) error {
	s, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return domain.ErrNotFound
	}
	if s.UserID != userID {
		return domain.ErrNotFound
	}
	return a.sessions.Revoke(ctx, sessionID)
}

func (a *Auth) Logout(ctx context.Context, sessionID string) error {
	return a.sessions.Revoke(ctx, sessionID)
}

func (a *Auth) LogoutAll(ctx context.Context, userID int64) error {
	return a.sessions.RevokeAll(ctx, userID, "")
}

func (a *Auth) Authorize(ctx context.Context, token string) (userID int64, roles []string, sessionID string, err error) {
	id, roles, sid, err := a.tokens.ParseAccess(token)
	if err != nil {
		return 0, nil, "", err
	}
	if _, err := a.sessions.Get(ctx, sid); err != nil {
		return 0, nil, "", domain.ErrInvalidToken
	}
	return id, roles, sid, nil
}

func (a *Auth) Me(ctx context.Context, id int64) (*domain.User, error) {
	return a.users.ByID(ctx, id)
}

func (a *Auth) SetAvatar(ctx context.Context, id int64, url string) (*domain.User, error) {
	if err := a.users.UpdateAvatar(ctx, id, url); err != nil {
		return nil, err
	}
	return a.users.ByID(ctx, id)
}

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
	return a.sessions.RevokeAll(ctx, id, "")
}

func (a *Auth) UpdateProfile(ctx context.Context, id int64, in UpdateProfileInput) (*domain.User, error) {
	if err := a.users.UpdateProfile(ctx, id, in.Username, strings.ToLower(in.Email), in.AvatarURL); err != nil {
		return nil, err
	}
	return a.users.ByID(ctx, id)
}
