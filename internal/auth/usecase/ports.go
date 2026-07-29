package usecase

import "context"

// The only port left. Two implementations pick between themselves at startup:
// mail.Log prints to the server log when SMTP_HOST is empty, mail.SMTP sends for
// real once it is set. Every other dependency of Auth is a concrete type.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}
