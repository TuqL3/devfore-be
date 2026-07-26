package domain

import "time"

type Status string

const (
	StatusActive Status = "active"
	StatusBanned Status = "banned"
)

const RoleStudent = "student"

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

func (u *User) HasPassword() bool { return u.PasswordHash != nil }
