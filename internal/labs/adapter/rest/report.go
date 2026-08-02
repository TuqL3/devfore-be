package rest

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/labs/domain"
)

type historyRow struct {
	SessionID  string     `json:"session_id"`
	LabTitle   string     `json:"lab_title"`
	LabSlug    string     `json:"lab_slug"`
	CourseSlug string     `json:"course_slug"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at"`
	Correct    int        `json:"correct"`
	Total      int        `json:"total"`
}

// History lists the caller's own attempts. No pagination: a student's history is
// their own labs, and the list is short enough that a page cursor would be more
// machinery than rows.
func (h *Handler) History(c *gin.Context) {
	rows, err := h.uc.History(c.Request.Context(), userID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	out := make([]historyRow, len(rows))
	for i, r := range rows {
		out[i] = historyRow{
			SessionID:  r.SessionID,
			LabTitle:   r.LabTitle,
			LabSlug:    r.LabSlug,
			CourseSlug: r.CourseSlug,
			Status:     string(r.Status),
			StartedAt:  r.StartedAt,
			EndedAt:    r.EndedAt,
			Correct:    r.Correct,
			Total:      r.Total,
		}
	}
	c.JSON(http.StatusOK, out)
}

// reportAnswer carries the answer key. Only ever built for a session that has
// ended — see Report and Submit, which are the two ways to reach it.
type reportAnswer struct {
	TaskID     int64      `json:"task_id"`
	Title      string     `json:"title"`
	Kind       string     `json:"kind"`
	Points     int        `json:"points"`
	Options    []string   `json:"options"`
	Correct    []int      `json:"correct"`
	Selected   []int      `json:"selected"`
	Passed     *bool      `json:"passed"`
	AnsweredAt *time.Time `json:"answered_at"`
	Attempts   *int       `json:"attempts"`
}

type reportResponse struct {
	SessionID   string         `json:"session_id"`
	LabTitle    string         `json:"lab_title"`
	LabSlug     string         `json:"lab_slug"`
	CourseSlug  string         `json:"course_slug"`
	Status      string         `json:"status"`
	StartedAt   time.Time      `json:"started_at"`
	EndedAt     *time.Time     `json:"ended_at"`
	SubmittedAt *time.Time     `json:"submitted_at"`
	Correct     int            `json:"correct"`
	Total       int            `json:"total"`
	Answers     []reportAnswer `json:"answers"`
}

func newReport(r *domain.Report) reportResponse {
	answers := make([]reportAnswer, len(r.Answers))
	for i, a := range r.Answers {
		answers[i] = reportAnswer{
			TaskID:     a.TaskID,
			Title:      a.Title,
			Kind:       a.Kind,
			Points:     a.Points,
			Options:    a.Options,
			Correct:    a.Correct,
			Selected:   a.Selected,
			Passed:     a.Passed,
			AnsweredAt: a.AnsweredAt,
			Attempts:   a.Attempts,
		}
	}
	return reportResponse{
		SessionID:   r.SessionID,
		LabTitle:    r.LabTitle,
		LabSlug:     r.LabSlug,
		CourseSlug:  r.CourseSlug,
		Status:      string(r.Status),
		StartedAt:   r.StartedAt,
		EndedAt:     r.EndedAt,
		SubmittedAt: r.SubmittedAt,
		Correct:     r.Correct,
		Total:       r.Total,
		Answers:     answers,
	}
}

// Submit ends the session and answers with the report, so handing in is one
// round trip rather than a submit followed by a fetch that could fail on its own.
func (h *Handler) Submit(c *gin.Context) {
	rep, err := h.uc.Submit(c.Request.Context(), c.Param("id"), userID(c))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case errors.Is(err, domain.ErrNotRunning):
		abort(c, http.StatusConflict, "phiên này đã kết thúc rồi")
	case errors.Is(err, domain.ErrIncomplete):
		abort(c, http.StatusConflict, "làm đúng hết các nhiệm vụ rồi mới nộp được")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, newReport(rep))
	}
}

func (h *Handler) Report(c *gin.Context) {
	rep, err := h.uc.Report(c.Request.Context(), c.Param("id"), userID(c))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case errors.Is(err, domain.ErrStillRunning):
		// The key is in this response, and the student is still in the lab
		// working it out.
		abort(c, http.StatusConflict, "phiên này chưa kết thúc")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, newReport(rep))
	}
}
