package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/devforge/be/internal/auth/adapter/totp"
	"github.com/devforge/be/internal/auth/domain"
)

// How long a half-finished login stays valid. Long enough to open an
// authenticator app and read a number, short enough that a token left in a
// browser history is worthless by the time anyone finds it.
const challengeTTL = 5 * time.Minute

// A code is six digits, so the guessing space is a million and a patient
// attacker with a live challenge is the threat. Challenges are single-use, which
// caps one token at one guess; this caps how fast new ones can be obtained for
// the same account.
const (
	totpAttemptLimit  = 10
	totpAttemptWindow = 15 * time.Minute
)

// TOTPStatus is what the security screen reads.
type TOTPStatus struct {
	Enabled bool
	// Codes left unused. Zero with Enabled true means the next lost phone is a
	// support ticket, which is worth saying out loud on screen.
	RecoveryLeft int
}

func (a *Auth) TOTPStatus(ctx context.Context, userID int64) (TOTPStatus, error) {
	t, err := a.users.TOTPSecret(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return TOTPStatus{}, nil
	}
	if err != nil {
		return TOTPStatus{}, err
	}
	if !t.Confirmed() {
		return TOTPStatus{}, nil
	}
	n, err := a.users.CountUnusedRecoveryCodes(ctx, userID)
	if err != nil {
		return TOTPStatus{}, err
	}
	return TOTPStatus{Enabled: true, RecoveryLeft: n}, nil
}

// StartTOTP generates a secret and returns it with the otpauth URI to scan. The
// factor is not on yet: nothing changes about logging in until a code proves the
// authenticator actually holds this secret.
func (a *Auth) StartTOTP(ctx context.Context, userID int64) (Enrolment, error) {
	u, err := a.users.ByID(ctx, userID)
	if err != nil {
		return Enrolment{}, err
	}
	secret, err := totp.NewSecret()
	if err != nil {
		return Enrolment{}, err
	}
	if err := a.users.StartTOTP(ctx, userID, secret); err != nil {
		return Enrolment{}, err
	}
	uri := totp.URI("DevForge", u.Email, secret)
	png, err := totp.QR(uri)
	if err != nil {
		return Enrolment{}, err
	}
	return Enrolment{Secret: secret, URI: uri, QR: png}, nil
}

// Enrolment is everything the setup screen needs to show. All three describe the
// same secret: the QR to scan, the URI to tap on a phone, and the key to type
// when neither is possible.
type Enrolment struct {
	Secret string
	URI    string
	// PNG data URI, ready for an img src.
	QR string
}

// ConfirmTOTP turns the factor on and hands back the recovery codes. They are
// returned once, here, and never again: they are stored hashed, so this is the
// only moment anything can read them.
func (a *Auth) ConfirmTOTP(ctx context.Context, userID int64, code string) ([]string, error) {
	t, err := a.users.TOTPSecret(ctx, userID)
	if err != nil {
		return nil, err
	}
	if t.Confirmed() {
		return nil, domain.ErrTOTPEnabled
	}
	if !totp.Validate(t.Secret, code) {
		return nil, domain.ErrInvalidCode
	}

	codes := make([]string, domain.RecoveryCodeCount)
	hashes := make([]string, domain.RecoveryCodeCount)
	for i := range codes {
		codes[i], err = newRecoveryCode()
		if err != nil {
			return nil, err
		}
		hashes[i], err = a.hasher.Hash(codes[i])
		if err != nil {
			return nil, err
		}
	}
	if err := a.users.ConfirmTOTP(ctx, userID, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

// DisableTOTP takes the factor off. The current password is required: an
// unattended session is exactly the situation a second factor exists for, and
// letting one switch it off would undo the point of having it.
func (a *Auth) DisableTOTP(ctx context.Context, userID int64, password string) error {
	u, err := a.users.ByID(ctx, userID)
	if err != nil {
		return err
	}
	// An account that only ever signed in with Google has no password to check.
	// Its second factor comes off on the strength of the live session, which is
	// the same strength Google's own sign-in gave it.
	if u.HasPassword() && !a.hasher.Check(*u.PasswordHash, password) {
		return domain.ErrCredentials
	}
	if err := a.users.DisableTOTP(ctx, userID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrTOTPNotEnabled
		}
		return err
	}
	return nil
}

// CompleteLogin finishes a login that stopped for a second factor. The challenge
// is spent on the way in whatever happens next, so one token buys one guess.
func (a *Auth) CompleteLogin(
	ctx context.Context, token, code string, meta domain.SessionMeta,
) (AuthOutput, error) {
	userID, err := a.codes.ConsumeChallenge(ctx, token)
	if err != nil {
		return AuthOutput{}, err
	}
	// Rate limited per account rather than per address: the attacker here holds
	// the password already and can ask for a fresh challenge whenever they like,
	// so the thing worth counting is guesses against this account.
	if err := a.codes.Attempt(ctx,
		fmt.Sprintf("2fa:%d", userID), totpAttemptLimit, totpAttemptWindow); err != nil {
		return AuthOutput{}, err
	}

	u, err := a.users.ByID(ctx, userID)
	if err != nil {
		return AuthOutput{}, err
	}
	// Re-checked rather than trusted from the password step: an account banned
	// in the minutes between the two must not get a session out of it.
	if u.IsBanned() {
		return AuthOutput{}, domain.ErrBanned
	}

	t, err := a.users.TOTPSecret(ctx, userID)
	if err != nil || !t.Confirmed() {
		return AuthOutput{}, domain.ErrTOTPNotEnabled
	}

	if !totp.Validate(t.Secret, code) {
		// Not a TOTP code, so it may be a recovery code. Tried second because
		// spending one is destructive and the authenticator is the normal path.
		if err := a.spendRecovery(ctx, userID, code); err != nil {
			return AuthOutput{}, err
		}
	}

	_ = a.codes.ClearAttempts(ctx, fmt.Sprintf("2fa:%d", userID))
	return a.start(ctx, u, meta)
}

// spendRecovery matches a submission against the unused codes and burns the one
// that fits. Every candidate is checked even after a match: bcrypt is slow and
// stopping early would say, in wall-clock, how far down the list the code was.
func (a *Auth) spendRecovery(ctx context.Context, userID int64, code string) error {
	rows, err := a.users.UnusedRecoveryCodes(ctx, userID)
	if err != nil {
		return err
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	var matched int64
	for _, row := range rows {
		if a.hasher.Check(row.CodeHash, code) {
			matched = row.ID
		}
	}
	if matched == 0 {
		return domain.ErrInvalidCode
	}
	return a.users.SpendRecoveryCode(ctx, matched)
}

// newRecoveryCode returns something a person can read off a screen and type
// back. Base32 without the padding, uppercase, split in the middle — no I, O, 1
// or 0 confusion to resolve because base32's alphabet has already dropped them.
func newRecoveryCode() (string, error) {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("recovery code: %w", err)
	}
	s := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return s[:8] + "-" + s[8:], nil
}
