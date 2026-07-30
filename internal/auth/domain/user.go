package domain

import "time"

type Status string

const (
	StatusActive Status = "active"
	// Signed up but has not entered the emailed code yet. The row exists so the
	// username and email are held, but it cannot log in.
	StatusPending Status = "pending"
	StatusBanned  Status = "banned"
)

const (
	RoleStudent = "student"
	// The only role that reaches the admin screens. Named here so the middleware
	// and the account-creation command cannot disagree on the spelling.
	RoleAdmin = "admin"
)

type User struct {
	ID           int64
	Username     string
	Email        string
	PasswordHash *string
	GoogleID     *string
	AvatarURL    *string
	Status       Status
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Roles        []string
}

func (u *User) IsBanned() bool { return u.Status == StatusBanned }

func (u *User) IsPending() bool { return u.Status == StatusPending }

func (u *User) HasPassword() bool { return u.PasswordHash != nil }

// GoogleProfile is what the OAuth adapter returns. Same reason as SessionMeta:
// the adapter has to name the type, so the type cannot live above it.
type GoogleProfile struct {
	ProviderID string
	Email      string
	Name       string
	AvatarURL  string
}
