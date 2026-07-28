// Package mail sends the two transactional emails the auth flows need. It is
// deliberately thin: net/smtp already speaks to every provider worth using
// (Gmail, Mailtrap, SES, Postmark) once you have a host and an app password.
package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	netmail "net/mail"
	"net/smtp"
	"strings"
)

// Log is the fallback when SMTP_HOST is unset. Printing the mail is what makes
// a fresh clone usable: `docker compose up`, register, read the code off the
// server log. Never selected in production — module.go picks SMTP once a host
// is configured.
type Log struct{}

func (Log) Send(_ context.Context, to, subject, body string) error {
	slog.Info("email not sent: SMTP_HOST is empty, printing instead",
		"to", to, "subject", subject, "body", body)
	return nil
}

type SMTP struct {
	addr string
	auth smtp.Auth
	// header is what the recipient sees and may carry a display name; envelope
	// is the bare address for the SMTP MAIL FROM command, which rejects
	// anything else with "501 invalid FROM parameter".
	header   string
	envelope string
}

func NewSMTP(host, port, user, pass, from string) *SMTP {
	var auth smtp.Auth
	// A local catcher (Mailhog, Mailpit) accepts mail with no credentials, and
	// handing it a PlainAuth would make it refuse the connection.
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	return &SMTP{addr: host + ":" + port, auth: auth, header: from, envelope: envelopeOf(from)}
}

// Accepts both MAIL_FROM styles: a bare "no-reply@x" and the friendlier
// "DevForge <no-reply@x>". An unparseable value is passed through so the SMTP
// server gets to report it, rather than being silently rewritten here.
func envelopeOf(from string) string {
	if a, err := netmail.ParseAddress(from); err == nil {
		return a.Address
	}
	return from
}

// The context is accepted for symmetry with the rest of the ports and because
// callers already have one; net/smtp predates context and offers no hook for
// it, so cancellation does not reach the socket.
// ponytail: switch to a library with a dialer (or wrap in a goroutine + timeout)
// if a hung SMTP server ever holds a request open.
func (s *SMTP) Send(_ context.Context, to, subject, body string) error {
	msg, err := build(s.header, to, subject, body)
	if err != nil {
		return err
	}
	if err := smtp.SendMail(s.addr, s.auth, s.envelope, []string{to}, msg); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}

// Header values are the injection surface here: a newline in the address would
// let a caller append headers of their own and turn one mail into many.
var errHeaderInjection = errors.New("mail: newline in header value")

func build(from, to, subject, htmlBody string) ([]byte, error) {
	if strings.ContainsAny(from+to+subject, "\r\n") {
		return nil, errHeaderInjection
	}
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	// Vietnamese subjects are not ASCII, and a raw UTF-8 header shows up as
	// mojibake in most clients.
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String()), nil
}
