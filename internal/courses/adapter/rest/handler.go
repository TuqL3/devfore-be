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
	// Where cover images are written and the origin they are served from. Empty
	// in a deployment with no upload directory, which the upload route reports
	// rather than silently writing nowhere.
	uploadDir string
	publicURL string
}

func NewHandler(uc *usecase.Courses, uploadDir, publicURL string) *Handler {
	return &Handler{uc: uc, uploadDir: uploadDir, publicURL: publicURL}
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

// Drills is the War Room list. Public like the course list is: what the platform
// offers has to be readable before signing up, or the answer to "đăng ký để dùng
// cái gì?" is a login wall.
func (h *Handler) Drills(c *gin.Context) {
	labs, err := h.uc.Drills(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	out := make([]labResponse, len(labs))
	for i := range labs {
		out[i] = newLab(&labs[i])
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) Drill(c *gin.Context) {
	lab, err := h.uc.Drill(c.Request.Context(), c.Param("slug"))
	switch {
	case errors.Is(err, domain.ErrLabNotFound):
		abort(c, http.StatusNotFound, "thử thách không tồn tại")
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

// MyCourses is the shelf on a student's own profile. Scoped to the caller by the
// session rather than by a path parameter: there is no id anybody can send that
// reads somebody else's enrolments.
func (h *Handler) MyCourses(c *gin.Context) {
	items, err := h.uc.MyCourses(c.Request.Context(), userID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, newEnrollmentList(items))
}
