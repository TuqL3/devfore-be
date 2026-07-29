package rest

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/courses/domain"
	"github.com/devforge/be/internal/courses/usecase"
)

type Handler struct {
	uc *usecase.Courses
}

func NewHandler(uc *usecase.Courses) *Handler {
	return &Handler{uc: uc}
}

func (h *Handler) List(c *gin.Context) {
	items, err := h.uc.List(c.Request.Context(), domain.CourseFilter{
		Level: c.Query("level"),
		Query: c.Query("q"),
	})
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, newSummaryList(items))
}

func (h *Handler) Levels(c *gin.Context) {
	levels, err := h.uc.Levels(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, newLevelList(levels))
}

func (h *Handler) Detail(c *gin.Context) {
	course, err := h.uc.Detail(c.Request.Context(), c.Param("slug"), userID(c))
	if errors.Is(err, domain.ErrNotFound) {
		abort(c, http.StatusNotFound, "khoá học không tồn tại")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, newDetail(course))
}

func (h *Handler) Lab(c *gin.Context) {
	lab, err := h.uc.Lab(c.Request.Context(), c.Param("slug"), c.Param("labSlug"))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "khoá học không tồn tại")
	case errors.Is(err, domain.ErrLabNotFound):
		abort(c, http.StatusNotFound, "bài học không tồn tại")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, newLabDetail(lab))
	}
}

func (h *Handler) Enroll(c *gin.Context) {
	err := h.uc.Enroll(c.Request.Context(), userID(c), c.Param("slug"))
	if errors.Is(err, domain.ErrNotFound) {
		abort(c, http.StatusNotFound, "khoá học không tồn tại")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Unenroll(c *gin.Context) {
	err := h.uc.Unenroll(c.Request.Context(), userID(c), c.Param("slug"))
	if errors.Is(err, domain.ErrNotFound) {
		abort(c, http.StatusNotFound, "khoá học không tồn tại")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Reviews(c *gin.Context) {
	items, err := h.uc.Reviews(c.Request.Context(), c.Param("slug"))
	if errors.Is(err, domain.ErrNotFound) {
		abort(c, http.StatusNotFound, "khoá học không tồn tại")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, newReviewList(items))
}

func (h *Handler) Leaderboard(c *gin.Context) {
	rows, err := h.uc.Leaderboard(c.Request.Context(), c.Param("slug"))
	if errors.Is(err, domain.ErrNotFound) {
		abort(c, http.StatusNotFound, "khoá học không tồn tại")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, newLeaderboard(rows))
}

func (h *Handler) Status(c *gin.Context) {
	course, err := h.uc.Detail(c.Request.Context(), c.Param("slug"), userID(c))
	if errors.Is(err, domain.ErrNotFound) {
		abort(c, http.StatusNotFound, "khoá học không tồn tại")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"lab_count":     course.LabCount,
		"student_count": course.StudentCount,
		"level":         course.Level,
		"enrolled":      course.Enrolled,
		"published_at":  course.PublishedAt,
		"updated_at":    course.UpdatedAt,
	})
}
