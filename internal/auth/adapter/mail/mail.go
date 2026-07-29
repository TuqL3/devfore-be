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

type Log struct{}

func (Log) Send(_ context.Context, to, subject, body string) error {
	slog.Info("email not sent: SMTP_HOST is empty, printing instead",
		"to", to, "subject", subject, "body", body)
	return nil
}

type SMTP struct {
	addr     string
	auth     smtp.Auth
	header   string
	envelope string
}

func NewSMTP(host, port, user, pass, from string) *SMTP {
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	return &SMTP{addr: host + ":" + port, auth: auth, header: from, envelope: envelopeOf(from)}
}

func envelopeOf(from string) string {
	if a, err := netmail.ParseAddress(from); err == nil {
		return a.Address
	}
	return from
}

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

var errHeaderInjection = errors.New("mail: newline in header value")

func build(from, to, subject, htmlBody string) ([]byte, error) {
	if strings.ContainsAny(from+to+subject, "\r\n") {
		return nil, errHeaderInjection
	}
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String()), nil
}
