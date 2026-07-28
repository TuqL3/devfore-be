package rest

import (
	"time"

	"github.com/devforge/be/internal/auth/domain"
)

type userResponse struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	AvatarURL *string   `json:"avatar_url"`
	Status    string    `json:"status"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
}

func newUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		AvatarURL: u.AvatarURL,
		Status:    string(u.Status),
		Roles:     u.Roles,
		CreatedAt: u.CreatedAt,
	}
}

// No token response type any more: a successful login answers with the user
// and nothing else. The credentials leave over Set-Cookie, where the page
// cannot reach them.

type sessionResponse struct {
	ID string `json:"id"`
	// The raw header. Turning it into "Chrome trên macOS" is the frontend's
	// job — it is presentation, and it changes far more often than this code.
	UserAgent string    `json:"user_agent"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	// Marks the device doing the asking, so the UI can label it and warn before
	// signing it out.
	Current bool `json:"current"`
}

func newSessionsResponse(list []domain.Session, currentID string) []sessionResponse {
	// Never nil: an empty JSON array is easier to consume than null.
	out := make([]sessionResponse, 0, len(list))
	for _, s := range list {
		out = append(out, sessionResponse{
			ID:        s.ID,
			UserAgent: s.UserAgent,
			IP:        s.IP,
			CreatedAt: s.CreatedAt,
			LastSeen:  s.LastSeen,
			Current:   s.ID == currentID,
		})
	}
	return out
}
