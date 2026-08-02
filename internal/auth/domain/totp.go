package domain

import "time"

// TOTP is an account's second factor as stored.
type TOTP struct {
	Secret string
	// Nil while enrolment is half done. Only a confirmed factor is asked for at
	// login, so a person who scanned a QR code and closed the tab is not locked
	// out of their own account.
	ConfirmedAt *time.Time
}

func (t *TOTP) Confirmed() bool { return t != nil && t.ConfirmedAt != nil }

// RecoveryCode is one unused code, with the id needed to spend it.
type RecoveryCode struct {
	ID       int64
	CodeHash string
}

// RecoveryCodeCount is how many codes are handed out at enrolment. Ten is enough
// that losing a phone is survivable and few enough that the list stays something
// a person will actually write down.
const RecoveryCodeCount = 10

// Challenge is the half-finished login handed back when an account has a second
// factor. It proves the password step was passed without being a session: it
// authorises exactly one thing, which is submitting a code.
type Challenge struct {
	Token string
	TTL   time.Duration
}
