package rest

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/i18n"
)

func bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		abort(c, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

// Every message a handler writes passes through here, which is why the handlers
// themselves are still written in one language. `msg` is the Vietnamese sentence
// and doubles as the catalogue key — see internal/i18n.
func abort(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": i18n.Msg(c, msg)})
}

// Same as abort plus a stable token the client can branch on. Only worth it
// where two failures share a status and lead to different screens — matching on
// the Vietnamese message would break the day someone reworded it.
func abortCode(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": i18n.Msg(c, msg), "code": code})
}

func serverError(c *gin.Context, err error) {
	slog.Error("handler error", "path", c.FullPath(), "err", err)
	abort(c, http.StatusInternalServerError, "lỗi máy chủ")
}

func newState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
