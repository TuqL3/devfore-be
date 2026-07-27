package usecase

import (
	"context"

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

type TokenIssuer interface {
	Issue(userID int64, roles []string) (domain.TokenPair, error)
	ParseAccess(token string) (userID int64, roles []string, err error)
	ParseRefresh(token string) (userID int64, err error)
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
