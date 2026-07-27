package usecase

import (
	"context"
	"strings"

	"github.com/devforge/be/internal/auth/domain"
)

type Auth struct {
	users  UserRepository
	tokens TokenIssuer
	hasher PasswordHasher
	oauth  OAuthExchanger
}

func NewAuth(users UserRepository, tokens TokenIssuer, hasher PasswordHasher, oauth OAuthExchanger) *Auth {
	return &Auth{users: users, tokens: tokens, hasher: hasher, oauth: oauth}
}

func (a *Auth) Authorize(token string) (userID int64, roles []string, err error) {
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

func (a *Auth) ChangePassword(ctx context.Context, id int64, current, next string) error {
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
	return a.users.UpdatePassword(ctx, id, hash)
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
	return a.users.Delete(ctx, id)
}

func (a *Auth) UpdateProfile(ctx context.Context, id int64, in UpdateProfileInput) (*domain.User, error) {
	if err := a.users.UpdateProfile(ctx, id, in.Username, strings.ToLower(in.Email), in.AvatarURL); err != nil {
		return nil, err
	}
	return a.users.ByID(ctx, id)
}
