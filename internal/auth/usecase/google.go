package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/devforge/be/internal/auth/domain"
)

func (a *Auth) OAuthEnabled() bool { return a.oauth.Enabled() }

func (a *Auth) OAuthURL(state string) string { return a.oauth.AuthCodeURL(state) }

func (a *Auth) AuthenticateGoogle(ctx context.Context, code string, meta SessionMeta) (AuthOutput, error) {
	p, err := a.oauth.Exchange(ctx, code)
	if err != nil {
		return AuthOutput{}, err
	}
	u, err := a.upsertGoogle(ctx, p)
	if err != nil {
		return AuthOutput{}, err
	}
	if u.IsBanned() {
		return AuthOutput{}, domain.ErrBanned
	}
	return a.start(ctx, u, meta)
}

func (a *Auth) upsertGoogle(ctx context.Context, p GoogleProfile) (*domain.User, error) {
	if u, err := a.users.ByGoogleID(ctx, p.ProviderID); err == nil {
		return u, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	email := strings.ToLower(p.Email)
	if u, err := a.users.ByEmail(ctx, email); err == nil {
		if err := a.users.LinkGoogle(ctx, u.ID, p.ProviderID, p.AvatarURL); err != nil {
			return nil, err
		}
		return u, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	username, err := a.uniqueUsername(ctx, email)
	if err != nil {
		return nil, err
	}
	u := &domain.User{
		Username:  username,
		Email:     email,
		GoogleID:  &p.ProviderID,
		AvatarURL: &p.AvatarURL,
		Status:    domain.StatusActive,
	}
	if err := a.users.Create(ctx, u, domain.RoleStudent); err != nil {
		return nil, err
	}
	u.Roles = []string{domain.RoleStudent}
	return u, nil
}

func (a *Auth) uniqueUsername(ctx context.Context, email string) (string, error) {
	base := sanitizeUsername(strings.Split(email, "@")[0])
	if base == "" {
		base = "user"
	}
	candidate := base
	for i := 1; i < 100; i++ {
		taken, err := a.users.UsernameTaken(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s%d", base, i)
	}
	return base + randHex(4), nil
}

func sanitizeUsername(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
