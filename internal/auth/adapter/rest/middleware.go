package rest

import (
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/auth/usecase"
)

const (
	ctxUserID = "user_id"
	ctxRoles  = "roles"
)

type Middleware struct {
	auth *usecase.Auth
}

func NewMiddleware(auth *usecase.Auth) *Middleware {
	return &Middleware{auth: auth}
}

func (m *Middleware) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearer(c)
		if token == "" {
			abort(c, http.StatusUnauthorized, "missing token")
			return
		}
		id, roles, err := m.auth.Authorize(token)
		if err != nil {
			abort(c, http.StatusUnauthorized, "invalid token")
			return
		}
		c.Set(ctxUserID, id)
		c.Set(ctxRoles, roles)
		c.Next()
	}
}

func (m *Middleware) Optional() gin.HandlerFunc {
	return func(c *gin.Context) {
		if token := bearer(c); token != "" {
			if id, roles, err := m.auth.Authorize(token); err == nil {
				c.Set(ctxUserID, id)
				c.Set(ctxRoles, roles)
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

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return after
	}
	return ""
}
