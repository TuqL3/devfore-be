// Package upload writes user-supplied images to disk and hands back the URL they
// will be served from. It exists so that avatars and course covers agree on what
// an acceptable image is: two copies of a size limit and a content-type list are
// two places for one of them to be loosened by mistake.
package upload

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// MaxImageBytes is the ceiling for anything that arrives as an image. Large
// enough for a photograph, small enough that a handful of them cannot fill a
// disk while nobody is looking.
const MaxImageBytes = 2 << 20 // 2 MiB

var (
	ErrTooLarge        = errors.New("image exceeds the size limit")
	ErrUnsupportedType = errors.New("unsupported image type")
	ErrNotConfigured   = errors.New("upload directory is not configured")
)

// extByType is also the allowlist. The extension is chosen from what the bytes
// actually are, never from the name the client sent — that name is where
// "avatar.png.php" comes from.
var extByType = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// Saver writes into Dir and builds URLs from PublicURL. Subdir separates one
// kind of upload from another on disk; empty keeps files at the top level,
// which is where avatars already live.
type Saver struct {
	Dir       string
	PublicURL string
	Subdir    string
}

// SaveImage stores the upload and returns its public URL. The old file, if the
// caller is replacing one, is left alone: an orphaned image costs a few
// kilobytes, while deleting the wrong one costs a page its picture.
func (s Saver) SaveImage(fh *multipart.FileHeader) (string, error) {
	if s.Dir == "" {
		return "", ErrNotConfigured
	}
	if fh.Size > MaxImageBytes {
		return "", ErrTooLarge
	}

	src, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	// Sniff the type from the first block rather than trusting the declared
	// content type, which the client also writes.
	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	ext, ok := extByType[strings.Split(http.DetectContentType(head[:n]), ";")[0]]
	if !ok {
		return "", ErrUnsupportedType
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	dir := filepath.Join(s.Dir, s.Subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := randomName() + ext
	dst, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer dst.Close()

	// Limited even after the size check: Size is what the client claimed in the
	// multipart header, and the body is what it actually sends.
	if _, err := io.Copy(dst, io.LimitReader(src, MaxImageBytes)); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/uploads/%s",
		strings.TrimRight(s.PublicURL, "/"), path.Join(s.Subdir, name)), nil
}

// ExceedsLimit tells a too-large upload apart from a missing one. The body
// limiter trips while the multipart form is still being parsed, so the handler
// never reaches the size check and would otherwise report a 6 MB photo as a
// missing file.
func ExceedsLimit(err error) bool {
	var maxBytes *http.MaxBytesError
	return errors.As(err, &maxBytes)
}

func randomName() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
