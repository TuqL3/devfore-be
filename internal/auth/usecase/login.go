package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/devforge/be/internal/auth/domain"
)

const placeholderHash = "$2a$12$a476X7Dj7KRWNAtc.NLEFePa7s75QNTUkVU1GigsNSFMfMWmE3Ff."

// Bcrypt makes each guess expensive but not expensive enough to leave the door
// open forever. The cap is per source and generous, because an office behind
// one address is a normal thing and a lockout there is an outage.
const (
	loginAttemptLimit  = 20
	loginAttemptWindow = 15 * time.Minute
)

func (a *Auth) Login(ctx context.Context, in LoginInput, meta SessionMeta) (AuthOutput, error) {
	// Counted before the password is examined, so a wrong username costs an
	// attacker the same as a wrong password.
	if meta.IP != "" {
		if err := a.codes.Attempt(ctx, "login:"+meta.IP, loginAttemptLimit, loginAttemptWindow); err != nil {
			return AuthOutput{}, err
		}
	}

	u, err := a.users.ByLogin(ctx, strings.ToLower(in.Login))
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return AuthOutput{}, err
	}

	stored := placeholderHash
	if u != nil && u.HasPassword() {
		stored = *u.PasswordHash
	}
	if !a.hasher.Check(stored, in.Password) || u == nil || !u.HasPassword() {
		return AuthOutput{}, domain.ErrCredentials
	}

	if u.IsBanned() {
		return AuthOutput{}, domain.ErrBanned
	}
	// The password was right, so this is the owner — telling them the account is
	// unverified leaks nothing and is the only way they learn to go finish it.
	if u.IsPending() {
		return AuthOutput{}, domain.ErrNotVerified
	}

	// Getting in wipes the counter, so ordinary traffic from a shared address
	// never accumulates toward a lockout — only a run of failures does.
	if meta.IP != "" {
		_ = a.codes.ClearAttempts(ctx, "login:"+meta.IP)
	}
	return a.start(ctx, u, meta)
}
