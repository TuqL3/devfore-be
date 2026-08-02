package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/devforge/be/internal/auth/domain"
)

// ForgotPassword answers the same way whatever it finds. The caller is
// unauthenticated by definition, so any difference between "sent" and "no such
// account" would turn this into an address checker.
func (a *Auth) ForgotPassword(ctx context.Context, email string) error {
	email = strings.ToLower(email)
	if err := a.codes.Throttle(ctx, "forgot:"+email, a.verify.Resend); err != nil {
		return err
	}

	u, err := a.users.ByEmail(ctx, email)
	if err != nil || u.IsBanned() {
		return nil
	}
	// Google-only accounts have no password to reset. Silence would read as a
	// mail that never arrived, so say what happened — the address is already
	// known to be theirs.
	if !u.HasPassword() {
		return a.mailer.Send(ctx, email, "Đặt lại mật khẩu DevForge", googleOnlyEmail())
	}

	token, err := newOpaqueToken()
	if err != nil {
		return err
	}
	if err := a.codes.PutReset(ctx, token, u.ID, a.verify.ResetTTL); err != nil {
		return err
	}
	link := a.verify.AppURL + "/reset-password?token=" + url.QueryEscape(token)
	return a.mailer.Send(ctx, email, "Đặt lại mật khẩu DevForge", resetEmail(link, a.verify.ResetTTL))
}

func (a *Auth) ResetPassword(ctx context.Context, token, next string) error {
	userID, err := a.codes.ConsumeReset(ctx, token)
	if err != nil {
		return domain.ErrInvalidToken
	}

	hash, err := a.hasher.Hash(next)
	if err != nil {
		return err
	}
	if err := a.users.UpdatePassword(ctx, userID, hash); err != nil {
		return err
	}

	// Following the link is the same proof of ownership the signup code asks
	// for, so an account still waiting on its code is verified by this too —
	// otherwise it would sit locked out with a password it cannot use.
	if u, err := a.users.ByID(ctx, userID); err == nil && u.IsPending() {
		if err := a.users.SetStatus(ctx, userID, domain.StatusActive); err != nil {
			return err
		}
	}

	// A reset is what someone does when they suspect the account is not only
	// theirs. Every session predating it has to go, including their own.
	return a.sessions.RevokeAll(ctx, userID, "")
}

// newOpaqueToken is 32 random bytes, url-safe. Used for both the password reset
// link and the half-finished login challenge: neither says anything about the
// account it names, which is what lets redis be the only place that mapping
// lives.
func newOpaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
