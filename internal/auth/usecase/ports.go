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
	SetStatus(ctx context.Context, id int64, status domain.Status) error
	Delete(ctx context.Context, id int64) error
}

type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Short-lived secrets that must not outlive their use: the signup code and the
// password-reset token. Both are stored hashed, so a dump of the store is not a
// pile of working credentials.
type VerifyStore interface {
	// PutCode replaces any code already outstanding for the address — asking for
	// a new one invalidates the old.
	PutCode(ctx context.Context, email, code string, ttl time.Duration) error
	// CheckCode consumes the code on success. It returns ErrTooManyAttempts once
	// an address has burned through its guesses, so a 6-digit code cannot be
	// walked through.
	CheckCode(ctx context.Context, email, code string) error
	PutReset(ctx context.Context, token string, userID int64, ttl time.Duration) error
	// ConsumeReset is single-use: the token is gone whether or not the caller
	// goes on to change the password.
	ConsumeReset(ctx context.Context, token string) (int64, error)
	// Throttle returns ErrRateLimited if key was already used inside window.
	// Guards the endpoints that send mail on request.
	Throttle(ctx context.Context, key string, window time.Duration) error
	// Attempt counts one try against key and returns ErrRateLimited once more
	// than limit have landed inside window. Unlike Throttle it allows a burst,
	// which is what a human retyping a code needs.
	Attempt(ctx context.Context, key string, limit int, window time.Duration) error
	// ClearAttempts forgets a counter. Called after a success, so a shared
	// office address is not walked toward a lockout by people who all get in.
	ClearAttempts(ctx context.Context, key string) error
}

type TokenIssuer interface {
	IssueAccess(userID int64, roles []string, sessionID string) (token string, ttl time.Duration, err error)
	ParseAccess(token string) (userID int64, roles []string, sessionID string, err error)
}

type SessionStore interface {
	Create(ctx context.Context, userID int64, meta SessionMeta) (domain.Session, error)
	Get(ctx context.Context, id string) (domain.Session, error)
	List(ctx context.Context, userID int64) ([]domain.Session, error)
	Revoke(ctx context.Context, id string) error
	RevokeAll(ctx context.Context, userID int64, keep string) error
	TTL() time.Duration
}

type SessionMeta struct {
	UserAgent string
	IP        string
	CreatedAt time.Time
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
