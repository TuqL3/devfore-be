package usecase

import (
	"context"
	"time"

	"github.com/devforge/be/internal/auth/domain"
)

type UserRepository interface {
	Create(ctx context.Context, u *domain.User, role string) error
	ByID(ctx context.Context, id int64) (*domain.User, error)
	ByEmail(ctx context.Context, email string) (*domain.User, error)
	ByGoogleID(ctx context.Context, gid string) (*domain.User, error)
	ByLogin(ctx context.Context, login string) (*domain.User, error)
	LinkGoogle(ctx context.Context, id int64, googleID, avatarURL string) error
	UsernameTaken(ctx context.Context, name string) (bool, error)
	UpdateProfile(ctx context.Context, id int64, username, email string, avatarURL *string) error
	UpdatePassword(ctx context.Context, id int64, hash string) error
	UpdateAvatar(ctx context.Context, id int64, url string) error
	Delete(ctx context.Context, id int64) error
}

// TokenIssuer only deals with access tokens now. Refreshing is a session
// lookup rather than a signature check, so nothing here can grant a session.
// The token carries its own session id so that any authenticated request knows
// which session it belongs to — the refresh cookie is scoped to /api/auth and
// simply is not sent anywhere else.
type TokenIssuer interface {
	IssueAccess(userID int64, roles []string, sessionID string) (token string, ttl time.Duration, err error)
	ParseAccess(token string) (userID int64, roles []string, sessionID string, err error)
}

// SessionStore holds refresh sessions server-side. Keeping them out of the
// token is what makes revocation — and therefore "log out of every device" —
// possible at all.
type SessionStore interface {
	Create(ctx context.Context, userID int64, meta SessionMeta) (domain.Session, error)
	// Get reports domain.ErrInvalidToken for an unknown or expired id.
	Get(ctx context.Context, id string) (domain.Session, error)
	// List is what the "signed-in devices" screen reads. Newest first.
	List(ctx context.Context, userID int64) ([]domain.Session, error)
	Revoke(ctx context.Context, id string) error
	// RevokeAll drops every session of a user. A non-empty keep spares that one
	// session, so an action like changing a password can sign out the other
	// devices without signing out the device that asked for it.
	RevokeAll(ctx context.Context, userID int64, keep string) error
	TTL() time.Duration
}

// SessionMeta is what the HTTP layer knows about the caller and the store does
// not. CreatedAt is carried across a refresh so a device keeps showing when it
// first signed in, rather than resetting every time the session rotates.
type SessionMeta struct {
	UserAgent string
	IP        string
	CreatedAt time.Time // zero means "now"
}

type PasswordHasher interface {
	Hash(plain string) (string, error)
	Check(hash, plain string) bool
}

type OAuthExchanger interface {
	Enabled() bool
	AuthCodeURL(state string) string
	Exchange(ctx context.Context, code string) (GoogleProfile, error)
}
