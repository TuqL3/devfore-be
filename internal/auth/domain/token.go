package domain

import "time"

type Credentials struct {
	AccessToken string
	AccessTTL   time.Duration
	SessionID   string
	SessionTTL  time.Duration
}

type Session struct {
	ID        string
	UserID    int64
	UserAgent string
	IP        string
	CreatedAt time.Time
	LastSeen  time.Time
}

// SessionMeta lives here rather than in usecase because the session adapter
// takes it as an argument: keeping it in usecase would make the adapter import
// the package that now imports the adapter.
type SessionMeta struct {
	UserAgent string
	IP        string
	CreatedAt time.Time
}
