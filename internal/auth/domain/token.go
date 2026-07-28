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
