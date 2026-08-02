// Package totp implements the time-based one-time password of RFC 6238, which
// is what Google Authenticator, Aegis and 1Password all speak.
//
// Hand-written rather than pulled in: the whole of it is an HMAC over a counter,
// and every part it needs — hmac, sha1, base32 — is in the standard library. A
// dependency for sixty lines would be a dependency to audit, update and trust.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"

	"rsc.io/qr"
)

const (
	// Thirty seconds and six digits, because that is what the authenticator apps
	// assume when a QR code does not say otherwise.
	Period = 30 * time.Second
	Digits = 6

	// How many steps either side of now still count. One step of slack absorbs a
	// phone clock that drifted and a person who typed the code as it rolled
	// over; more than that starts widening the window an intercepted code is
	// useful in.
	skew = 1
)

// base32 without padding, which is the encoding every authenticator expects in
// an otpauth:// secret.
var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a fresh 20-byte secret, base32 encoded. Twenty bytes is the
// length RFC 4226 recommends for HMAC-SHA1.
func NewSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("totp secret: %w", err)
	}
	return enc.EncodeToString(b), nil
}

// Code computes the password for one moment. Exported for the test, which checks
// it against the vectors in RFC 6238.
func Code(secret string, at time.Time) (string, error) {
	key, err := enc.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", fmt.Errorf("totp secret: %w", err)
	}

	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(at.Unix())/uint64(Period.Seconds()))

	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)

	// Dynamic truncation, RFC 4226 §5.3: the low nibble of the last byte picks
	// where in the digest to read the number from.
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	mod := uint32(1)
	for range Digits {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", Digits, value%mod), nil
}

// Validate reports whether code is right for now, allowing one step of clock
// skew either side.
//
// The comparison is constant time. A timing difference here would leak how much
// of a guess was correct, which turns a million-guess space into six thousand.
func Validate(secret, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return false
	}
	now := time.Now()
	ok := false
	for i := -skew; i <= skew; i++ {
		want, err := Code(secret, now.Add(time.Duration(i)*Period))
		if err != nil {
			return false
		}
		// No early return: every step is checked whatever the first one said, so
		// the time taken does not say which step matched.
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			ok = true
		}
	}
	return ok
}

// QR renders an otpauth URI as a PNG data URI, ready to drop straight into an
// img tag. Returned inline rather than from an endpoint of its own: the secret
// is already in the response this goes into, so a separate URL would be a second
// place to get the authorisation right.
//
// The library does the Reed-Solomon and the mask selection. Hand-writing those
// is roughly three hundred lines of finite-field arithmetic whose failure mode
// is a code that scans on one phone and not the next — the kind of bug that
// reaches users before it reaches a test.
//
// Level M corrects around 15% damage, which is what every authenticator's own
// enrolment code uses. Higher levels make the image denser for no gain on a
// screen that is not going to be creased or photocopied.
func QR(uri string) (string, error) {
	code, err := qr.Encode(uri, qr.M)
	if err != nil {
		return "", fmt.Errorf("totp qr: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG()), nil
}

// URI builds the otpauth:// string an authenticator scans. The issuer appears
// twice on purpose — once as a label prefix and once as a parameter — because
// older apps read one and newer ones read the other.
func URI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(Digits))
	q.Set("period", fmt.Sprint(int(Period.Seconds())))
	return "otpauth://totp/" + label + "?" + q.Encode()
}
