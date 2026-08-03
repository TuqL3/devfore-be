package rest

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/courses/domain"
)

// courseInput is the admin form. image_url is a pointer so "field absent" and
// "field cleared" stay different things.
type courseInput struct {
	Slug        string  `json:"slug"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	ImageURL    *string `json:"image_url"`
	Level       string  `json:"level"`
	Status      string  `json:"status"`
}

func (in courseInput) toDomain() domain.CourseInput {
	return domain.CourseInput{
		Slug:        in.Slug,
		Title:       in.Title,
		Description: in.Description,
		ImageURL:    in.ImageURL,
		Level:       in.Level,
		Status:      in.Status,
	}
}

// AdminList shows drafts alongside published courses. Everything else about it
// is the public listing, filters included.
func (h *Handler) AdminList(c *gin.Context) {
	items, err := h.uc.ListAll(c.Request.Context(), domain.CourseFilter{
		Level: c.Query("level"),
		Query: c.Query("q"),
	})
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, newSummaryList(items))
}

func (h *Handler) AdminCreate(c *gin.Context) {
	var in courseInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	course, err := h.uc.Create(c.Request.Context(), in.toDomain())
	if written := writeCourseError(c, err); written {
		return
	}
	c.JSON(http.StatusCreated, newSummary(*course))
}

func (h *Handler) AdminUpdate(c *gin.Context) {
	id, ok := courseID(c)
	if !ok {
		return
	}
	var in courseInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	course, err := h.uc.Update(c.Request.Context(), id, in.toDomain())
	if written := writeCourseError(c, err); written {
		return
	}
	c.JSON(http.StatusOK, newSummary(*course))
}

func (h *Handler) AdminDelete(c *gin.Context) {
	id, ok := courseID(c)
	if !ok {
		return
	}
	err := h.uc.Delete(c.Request.Context(), id)
	if written := writeCourseError(c, err); written {
		return
	}
	c.Status(http.StatusNoContent)
}

func courseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		abort(c, http.StatusBadRequest, "id không hợp lệ")
		return 0, false
	}
	return id, true
}

// writeCourseError reports whether it answered the request. The three admin
// handlers fail in exactly the same ways, and repeating the switch in each was
// how the messages would drift apart.
func writeCourseError(c *gin.Context, err error) bool {
	var invalid domain.InvalidInput
	switch {
	case err == nil:
		return false
	case errors.As(err, &invalid):
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
			"error": invalid.Message,
			// The form highlights the offending input rather than making the
			// admin guess which of six fields the message is about.
			"field": invalid.Field,
		})
	case errors.Is(err, domain.ErrSlugTaken):
		abort(c, http.StatusConflict, "slug này đã được dùng cho khoá khác")
	case errors.Is(err, domain.ErrLabSlugTaken):
		// Lab slugs are unique table-wide, so the clash can be with a lab in a
		// course this admin is not even looking at.
		abort(c, http.StatusConflict, "slug này đã được dùng cho lab khác")
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "khoá học không tồn tại")
	case errors.Is(err, domain.ErrLabNotFound):
		abort(c, http.StatusNotFound, "lab không tồn tại")
	case errors.Is(err, domain.ErrTaskNotFound):
		abort(c, http.StatusNotFound, "nhiệm vụ không tồn tại")
	case errors.Is(err, domain.ErrReviewNotFound):
		abort(c, http.StatusNotFound, "bài ôn tập không tồn tại")
	default:
		serverError(c, err)
	}
	return true
}
