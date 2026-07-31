package rest

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/upload"
)

const avatarField = "file"

func (h *Handler) UploadAvatar(c *gin.Context) {
	// A little headroom over the limit so the multipart envelope itself does not
	// trip the reader before the size check can report a readable error.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, upload.MaxImageBytes+1024)
	fh, err := c.FormFile(avatarField)
	if err != nil {
		if upload.ExceedsLimit(err) {
			abort(c, http.StatusRequestEntityTooLarge, "ảnh vượt quá 2MB")
			return
		}
		abort(c, http.StatusBadRequest, "thiếu file ảnh")
		return
	}

	url, err := upload.Saver{Dir: h.uploadDir, PublicURL: h.publicURL}.SaveImage(fh)
	switch {
	case errors.Is(err, upload.ErrNotConfigured):
		abort(c, http.StatusNotImplemented, "chưa cấu hình nơi lưu ảnh")
		return
	case errors.Is(err, upload.ErrTooLarge):
		abort(c, http.StatusRequestEntityTooLarge, "ảnh vượt quá 2MB")
		return
	case errors.Is(err, upload.ErrUnsupportedType):
		abort(c, http.StatusUnsupportedMediaType, "chỉ nhận ảnh png, jpg, gif hoặc webp")
		return
	case err != nil:
		serverError(c, err)
		return
	}

	u, err := h.auth.SetAvatar(c.Request.Context(), UserID(c), url)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, newUserResponse(u))
}
