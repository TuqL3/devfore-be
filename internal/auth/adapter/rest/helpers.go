package rest

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

func bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		abort(c, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func abort(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}

// Same as abort plus a stable token the client can branch on. Only worth it
// where two failures share a status and lead to different screens — matching on
// the Vietnamese message would break the day someone reworded it.
func abortCode(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg, "code": code})
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
