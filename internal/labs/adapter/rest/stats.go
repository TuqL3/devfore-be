package rest

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/audit"
	"github.com/devforge/be/internal/labs/domain"
)

type labStat struct {
	LabID       int64  `json:"lab_id"`
	LabTitle    string `json:"lab_title"`
	CourseTitle string `json:"course_title"`
	Sessions    int    `json:"sessions"`
	Submitted   int    `json:"submitted"`
	Answered    int    `json:"answered"`
	Retried     int    `json:"retried"`
}

type statsResponse struct {
	Students       int       `json:"students"`
	ActiveStudents int       `json:"active_students"`
	Courses        int       `json:"courses"`
	Published      int       `json:"published"`
	Sessions       int       `json:"sessions"`
	SessionsWeek   int       `json:"sessions_week"`
	Running        int       `json:"running"`
	Submitted      int       `json:"submitted"`
	Labs           []labStat `json:"labs"`
}

type runningSession struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	LabTitle     string    `json:"lab_title"`
	StartedAt    time.Time `json:"started_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	HasContainer bool      `json:"has_container"`
}

// AdminRunningSessions lists every live container. Its own endpoint rather than
// a field on the stats response: killing one only has to reload this, and the
// overview's totals do not change often enough to refetch alongside it.
func (h *Handler) AdminRunningSessions(c *gin.Context) {
	rows, err := h.uc.RunningSessions(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	out := make([]runningSession, len(rows))
	for i, s := range rows {
		out[i] = runningSession(s)
	}
	c.JSON(http.StatusOK, out)
}

// AdminKillSession removes someone else's container. The student is not warned:
// there is no channel to warn them on, and the terminal closing is the message.
// Their answers are already on record — grading writes as each check runs, so
// this ends the attempt without throwing away what they had passed.
func (h *Handler) AdminKillSession(c *gin.Context) {
	id := c.Param("id")
	err := h.uc.AdminStop(c.Request.Context(), id)
	if err == nil {
		h.audit.Record(c, audit.Entry{
			ActorID:    userID(c),
			Action:     audit.ActionSessionKill,
			TargetType: audit.TargetSession,
			TargetID:   id,
		})
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case errors.Is(err, domain.ErrNotRunning):
		abort(c, http.StatusConflict, "phiên này đã kết thúc rồi")
	case err != nil:
		serverError(c, err)
	default:
		c.Status(http.StatusNoContent)
	}
}

// AdminStats is the overview screen's only request. Admin-only on the route:
// these are platform-wide figures, and no part of them is scoped to a caller.
func (h *Handler) AdminStats(c *gin.Context) {
	s, err := h.uc.Stats(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	labs := make([]labStat, len(s.Labs))
	for i, l := range s.Labs {
		labs[i] = labStat{
			LabID:       l.LabID,
			LabTitle:    l.LabTitle,
			CourseTitle: l.CourseTitle,
			Sessions:    l.Sessions,
			Submitted:   l.Submitted,
			Answered:    l.Answered,
			Retried:     l.Retried,
		}
	}
	c.JSON(http.StatusOK, statsResponse{
		Students:       s.Students,
		ActiveStudents: s.ActiveStudents,
		Courses:        s.Courses,
		Published:      s.Published,
		Sessions:       s.Sessions,
		SessionsWeek:   s.SessionsWeek,
		Running:        s.Running,
		Submitted:      s.Submitted,
		Labs:           labs,
	})
}
