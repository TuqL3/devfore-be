package rest

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/auth/domain"
)

const (
	accessCookie  = "df_access"
	sessionCookie = "df_session"

	// The refresh cookie is only ever needed by the endpoints that mint or end
	// a session, so it is not attached to any other request — a narrower path
	// means fewer places it can leak from.
	sessionCookiePath = "/api/auth"
)

// CookieConfig is the deployment-dependent half of cookie handling: whether
// TLS is in play, and whether the cookie has to span subdomains.
type CookieConfig struct {
	Domain string
	Secure bool
}

// setCredentials writes both halves of a login.
//
// HttpOnly on both: page scripts never need to read either value, and keeping
// them out of JavaScript is what makes an XSS bug stop short of stealing a
// session. SameSite=Lax means a cross-site form post arrives without them,
// which is the first line of CSRF defence — the Origin check in the HTTP layer
// is the second.
func setCredentials(c *gin.Context, cr domain.Credentials, cfg CookieConfig) {
	write(c, accessCookie, cr.AccessToken, "/", cr.AccessTTL, cfg)
	write(c, sessionCookie, cr.SessionID, sessionCookiePath, cr.SessionTTL, cfg)
}

// clearCredentials expires both cookies. Path and Domain must match what was
// set, or the browser keeps the originals and the user stays signed in.
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

// SessionID is the caller's session, or "" if there is none to be had.
//
// Two sources because the two live in different places: under /api/auth the
// refresh cookie is present and no access token has been parsed yet, while
// everywhere else the cookie is out of scope and the id rides in the access
// token instead.
func SessionID(c *gin.Context) string {
	if v, ok := c.Get(ctxSessionID); ok {
		if sid, _ := v.(string); sid != "" {
			return sid
		}
	}
	return cookie(c, sessionCookie)
}
