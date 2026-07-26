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

func serverError(c *gin.Context, err error) {
	slog.Error("handler error", "path", c.FullPath(), "err", err)
	abort(c, http.StatusInternalServerError, "lỗi máy chủ")
}

func newState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
