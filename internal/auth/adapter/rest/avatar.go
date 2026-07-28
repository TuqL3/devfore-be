package rest

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	maxAvatarBytes = 2 << 20 // 2 MiB
	avatarField    = "file"
)

var avatarExt = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

func (h *Handler) UploadAvatar(c *gin.Context) {
	if h.uploadDir == "" {
		abort(c, http.StatusNotImplemented, "chưa cấu hình nơi lưu ảnh")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAvatarBytes+1024)
	fh, err := c.FormFile(avatarField)
	if err != nil {
		abort(c, http.StatusBadRequest, "thiếu file ảnh")
		return
	}
	if fh.Size > maxAvatarBytes {
		abort(c, http.StatusRequestEntityTooLarge, "ảnh vượt quá 2MB")
		return
	}

	src, err := fh.Open()
	if err != nil {
		serverError(c, err)
		return
	}
	defer src.Close()

	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		serverError(c, err)
		return
	}
	ext, ok := avatarExt[strings.Split(http.DetectContentType(head[:n]), ";")[0]]
	if !ok {
		abort(c, http.StatusUnsupportedMediaType, "chỉ nhận ảnh png, jpg, gif hoặc webp")
		return
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		serverError(c, err)
		return
	}

	if err := os.MkdirAll(h.uploadDir, 0o755); err != nil {
		serverError(c, err)
		return
	}
	name := randomName() + ext
	dst, err := os.Create(filepath.Join(h.uploadDir, name))
	if err != nil {
		serverError(c, err)
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, io.LimitReader(src, maxAvatarBytes)); err != nil {
		serverError(c, err)
		return
	}

	url := fmt.Sprintf("%s/uploads/%s", strings.TrimRight(h.publicURL, "/"), name)
	u, err := h.auth.SetAvatar(c.Request.Context(), UserID(c), url)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, newUserResponse(u))
}

func randomName() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
