package usecase

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/devforge/be/internal/auth/domain"
)

// VerifyEmail turns a pending signup into a real account and signs it in. The
// code proves the address exists and belongs to whoever is at the keyboard, so
// there is nothing left to ask for.
// The per-address attempt cap burns one code after five wrong guesses, which
// stops nobody who is willing to request a fresh code each round. Counting per
// source as well is what makes that loop expensive, and it is checked before
// anything else so a wrong address costs the same as a right one.
const (
	ipAttemptLimit  = 20
	ipAttemptWindow = 15 * time.Minute
)

func (a *Auth) VerifyEmail(ctx context.Context, email, code string, meta SessionMeta) (AuthOutput, error) {
	email = strings.ToLower(email)

	if meta.IP != "" {
		if err := a.codes.Attempt(ctx, "verify:"+meta.IP, ipAttemptLimit, ipAttemptWindow); err != nil {
			return AuthOutput{}, err
		}
	}

	u, err := a.users.ByEmail(ctx, email)
	if err != nil {
		// Same error as a wrong code: whether an address is registered is not
		// something this endpoint should confirm to a stranger.
		return AuthOutput{}, domain.ErrInvalidCode
	}
	if u.IsBanned() {
		return AuthOutput{}, domain.ErrBanned
	}
	if !u.IsPending() {
		return AuthOutput{}, domain.ErrInvalidCode
	}

	if err := a.codes.CheckCode(ctx, email, code); err != nil {
		return AuthOutput{}, err
	}
	if err := a.users.SetStatus(ctx, u.ID, domain.StatusActive); err != nil {
		return AuthOutput{}, err
	}
	u.Status = domain.StatusActive
	return a.start(ctx, u, meta)
}

// ResendCode is deliberately quiet: it reports success for an unknown address
// too, so it cannot be used to test which emails have accounts. The throttle is
// what stops it being used to mail-bomb someone.
func (a *Auth) ResendCode(ctx context.Context, email string) error {
	email = strings.ToLower(email)
	if err := a.codes.Throttle(ctx, "resend:"+email, a.verify.Resend); err != nil {
		return err
	}
	u, err := a.users.ByEmail(ctx, email)
	if err != nil || !u.IsPending() {
		return nil
	}
	return a.sendCode(ctx, email)
}

func (a *Auth) sendCode(ctx context.Context, email string) error {
	code, err := newCode()
	if err != nil {
		return err
	}
	if err := a.codes.PutCode(ctx, email, code, a.verify.CodeTTL); err != nil {
		return err
	}
	return a.mailer.Send(ctx, email, "Mã xác thực DevForge", codeEmail(code, a.verify.CodeTTL))
}

// Six digits, uniformly drawn — %06d keeps the leading zeros that make it six
// digits every time instead of sometimes five.
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
