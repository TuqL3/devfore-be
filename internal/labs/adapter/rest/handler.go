package rest

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase"
)

type Handler struct {
	uc *usecase.Labs
}

func NewHandler(uc *usecase.Labs) *Handler { return &Handler{uc: uc} }

type sessionResponse struct {
	ID           string    `json:"id"`
	LabID        int64     `json:"lab_id"`
	Status       string    `json:"status"`
	StartedAt    time.Time `json:"started_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	SecondsLeft  int       `json:"seconds_left"`
	TerminalPath string    `json:"terminal_path"`
}

func newSessionResponse(s *domain.Session) sessionResponse {
	return sessionResponse{
		ID:          s.ID,
		LabID:       s.LabID,
		Status:      string(s.Status),
		StartedAt:   s.StartedAt,
		ExpiresAt:   s.ExpiresAt,
		SecondsLeft: int(usecase.Remaining(s).Seconds()),
		// Handed to the client rather than built there, so the route can move
		// without a second repository needing to be edited in step.
		TerminalPath: "/ws/terminal/" + s.ID,
	}
}

func (h *Handler) Start(c *gin.Context) {
	out, err := h.uc.Start(c.Request.Context(), userID(c), c.Param("slug"))
	switch {
	case errors.Is(err, domain.ErrLabNotFound):
		abort(c, http.StatusNotFound, "bài lab không tồn tại")
	case errors.Is(err, domain.ErrAlreadyRunning):
		abort(c, http.StatusConflict, "bạn đang có một phiên lab chạy dở, hãy đóng nó trước")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusCreated, newSessionResponse(out.Session))
	}
}

// Current lets a client that reloaded the page find its way back to the session
// it already owns, instead of pressing Start and being told it has one.
func (h *Handler) Current(c *gin.Context) {
	s, err := h.uc.Running(c.Request.Context(), userID(c))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusOK, gin.H{"session": nil})
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, gin.H{"session": newSessionResponse(s)})
	}
}

func (h *Handler) Session(c *gin.Context) {
	s, err := h.uc.Owned(c.Request.Context(), c.Param("id"), userID(c))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, newSessionResponse(s))
	}
}

func (h *Handler) Stop(c *gin.Context) {
	err := h.uc.Stop(c.Request.Context(), c.Param("id"), userID(c))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case errors.Is(err, domain.ErrNotRunning):
		// Already gone is the outcome the caller wanted.
		c.Status(http.StatusNoContent)
	case err != nil:
		serverError(c, err)
	default:
		c.Status(http.StatusNoContent)
	}
}

func abort(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}

func serverError(c *gin.Context, err error) {
	slog.Error("labs handler error", "path", c.FullPath(), "err", err)
	abort(c, http.StatusInternalServerError, "lỗi máy chủ")
}

func userID(c *gin.Context) int64 {
	v, _ := c.Get("user_id")
	id, _ := v.(int64)
	return id
}
