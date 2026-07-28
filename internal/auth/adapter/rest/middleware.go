package rest

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/auth/usecase"
)

const (
	ctxUserID    = "user_id"
	ctxRoles     = "roles"
	ctxSessionID = "session_id"
)

type Middleware struct {
	auth *usecase.Auth
}

func NewMiddleware(auth *usecase.Auth) *Middleware {
	return &Middleware{auth: auth}
}

func (m *Middleware) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := cookie(c, accessCookie)
		if token == "" {
			abort(c, http.StatusUnauthorized, "missing token")
			return
		}
		id, roles, sid, err := m.auth.Authorize(c.Request.Context(), token)
		if err != nil {
			abort(c, http.StatusUnauthorized, "invalid token")
			return
		}
		c.Set(ctxUserID, id)
		c.Set(ctxRoles, roles)
		c.Set(ctxSessionID, sid)
		c.Next()
	}
}

func (m *Middleware) Optional() gin.HandlerFunc {
	return func(c *gin.Context) {
		if token := cookie(c, accessCookie); token != "" {
			if id, roles, sid, err := m.auth.Authorize(c.Request.Context(), token); err == nil {
				c.Set(ctxUserID, id)
				c.Set(ctxRoles, roles)
				c.Set(ctxSessionID, sid)
			}
		}
		c.Next()
	}
}

func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roles, _ := c.Get(ctxRoles)
		rs, _ := roles.([]string)
		if !slices.Contains(rs, role) {
			abort(c, http.StatusForbidden, "forbidden")
			return
		}
		c.Next()
	}
}

func UserID(c *gin.Context) int64 {
	v, _ := c.Get(ctxUserID)
	id, _ := v.(int64)
	return id
}
