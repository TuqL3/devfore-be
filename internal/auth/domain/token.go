package domain

import "time"

// Credentials is what a successful authentication hands back: a short-lived
// access token, and the id of the server-side session allowed to mint new ones.
//
// Both travel as HttpOnly cookies, so neither is readable by page scripts.
// That is also why the refresh half is an opaque session id rather than a
// self-contained JWT: an id can be revoked, a signed token cannot.
type Credentials struct {
	AccessToken string
	AccessTTL   time.Duration
	SessionID   string
	SessionTTL  time.Duration
}

// Session is one signed-in device, as shown on the devices screen.
//
// CreatedAt is when that device signed in; LastSeen is when it last refreshed.
// They differ because a session rotates its id roughly every access-token
// lifetime while staying, to the person looking at the list, the same device.
type Session struct {
	ID        string
	UserID    int64
	UserAgent string
	IP        string
	CreatedAt time.Time
	LastSeen  time.Time
}
