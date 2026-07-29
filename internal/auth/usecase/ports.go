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

type VerifyStore interface {
	PutCode(ctx context.Context, email, code string, ttl time.Duration) error
	CheckCode(ctx context.Context, email, code string) error
	PutReset(ctx context.Context, token string, userID int64, ttl time.Duration) error
	ConsumeReset(ctx context.Context, token string) (int64, error)
	Throttle(ctx context.Context, key string, window time.Duration) error
	Attempt(ctx context.Context, key string, limit int, window time.Duration) error
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
