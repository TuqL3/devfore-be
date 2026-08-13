package rest

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/i18n"
)

// See the note on the auth package's copy: `msg` is both the Vietnamese
// sentence and the key it is translated by.
func abort(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": i18n.Msg(c, msg)})
}

func serverError(c *gin.Context, err error) {
	slog.Error("courses handler error", "path", c.FullPath(), "err", err)
	abort(c, http.StatusInternalServerError, "lỗi máy chủ")
}

func userID(c *gin.Context) int64 {
	v, _ := c.Get("user_id")
	id, _ := v.(int64)
	return id
}
