package rest

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/auth/domain"
)

const (
	accessCookie      = "df_access"
	sessionCookie     = "df_session"
	sessionCookiePath = "/api/auth"
)

type CookieConfig struct {
	Domain string
	Secure bool
}

func setCredentials(c *gin.Context, cr domain.Credentials, cfg CookieConfig) {
	write(c, accessCookie, cr.AccessToken, "/", cr.AccessTTL, cfg)
	write(c, sessionCookie, cr.SessionID, sessionCookiePath, cr.SessionTTL, cfg)
}

func clearCredentials(c *gin.Context, cfg CookieConfig) {
	write(c, accessCookie, "", "/", -time.Hour, cfg)
	write(c, sessionCookie, "", sessionCookiePath, -time.Hour, cfg)
}

func write(c *gin.Context, name, value, path string, ttl time.Duration, cfg CookieConfig) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Domain:   cfg.Domain,
		MaxAge:   int(ttl.Seconds()),
		Secure:   cfg.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func cookie(c *gin.Context, name string) string {
	v, err := c.Cookie(name)
	if err != nil {
		return ""
	}
	return v
}

func SessionID(c *gin.Context) string {
	if v, ok := c.Get(ctxSessionID); ok {
		if sid, _ := v.(string); sid != "" {
			return sid
		}
	}
	return cookie(c, sessionCookie)
}
