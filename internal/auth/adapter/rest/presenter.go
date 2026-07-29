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

type sessionResponse struct {
	ID        string    `json:"id"`
	UserAgent string    `json:"user_agent"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	Current   bool      `json:"current"`
}

func newSessionsResponse(list []domain.Session, currentID string) []sessionResponse {
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
