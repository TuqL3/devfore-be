package usecase

import (
	"context"

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
