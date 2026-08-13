package rest

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/audit"
	"github.com/devforge/be/internal/events"
	"github.com/devforge/be/internal/labs/domain"
)

// The admin read endpoints.
//
// All admin-only, all read-only, and two of them audited: reading somebody's
// activity and opening somebody's shell history are things done TO a person, and
// the platform already holds that whoever does one leaves a name behind.

// AdminOverview is the live screen in one request.
//
// `?since=` in hours, defaulting to 24. One window for every count on the strip,
// so the numbers are comparable with each other — a screen mixing "today" and
// "this week" invites exactly the wrong arithmetic.
func (h *Handler) AdminOverview(c *gin.Context) {
	hours, _ := strconv.Atoi(c.Query("hours"))
	if hours <= 0 || hours > 24*30 {
		hours = 24
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	ov, err := h.uc.Overview(c.Request.Context(), since)
	if err != nil {
		serverError(c, err)
		return
	}

	running := make([]runningSession, len(ov.Running))
	for i, s := range ov.Running {
		running[i] = runningSession(s)
	}

	// The event summary rides along: an admin opening this screen is asking "is
	// anything wrong", and a strip of counts that omits the failures answers half
	// the question.
	var summary []events.KindCount
	if h.events != nil {
		if rows, err := h.events.Summary(c.Request.Context(), since); err == nil {
			summary = rows
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"hours":     hours,
		"counts":    ov.Counts,
		"running":   running,
		"max_slots": ov.MaxSlots,
		"events":    summary,
	})
}

// AdminEvents is the failure feed. Filters are exact matches on what the summary
// already showed, so there is nothing to type and nothing to mistype.
func (h *Handler) AdminEvents(c *gin.Context) {
	if h.events == nil {
		c.JSON(http.StatusOK, gin.H{"events": []any{}})
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	rows, err := h.events.List(c.Request.Context(), c.Query("kind"), c.Query("severity"), limit)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": rows})
}

// AdminContentHealth is the analysis screen: which task, lab, scenario or course
// is not working.
func (h *Handler) AdminContentHealth(c *gin.Context) {
	out, err := h.uc.ContentHealth(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tasks":     out.Tasks,
		"labs":      out.Labs,
		"incidents": out.Incidents,
		"courses":   out.Courses,
	})
}

// AdminSharedReports answers "what is public right now" — the question the
// takedown box could not ask, because it only ever knew about links it was
// handed.
func (h *Handler) AdminSharedReports(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	rows, err := h.uc.SharedReports(c.Request.Context(), limit)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"reports": rows})
}

// AdminUserActivity is one person's page.
//
// Audited. This is reading what somebody else did — their attempts, and the
// pipelines they wrote — and the platform's rule is already that actions taken
// on a person leave a name behind. Reading is an action here: the pipelines are
// their work, not a system record.
func (h *Handler) AdminUserActivity(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		abort(c, http.StatusBadRequest, "id không hợp lệ")
		return
	}
	out, err := h.uc.UserActivity(c.Request.Context(), id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "không tìm thấy tài khoản")
	case err != nil:
		serverError(c, err)
	default:
		h.audit.Record(c, audit.Entry{
			ActorID:    userID(c),
			Action:     audit.ActionUserActivityRead,
			TargetType: audit.TargetUser,
			TargetID:   c.Param("id"),
		})
		c.JSON(http.StatusOK, gin.H{
			"summary":  out.Summary,
			"sessions": out.Sessions,
			"sim_runs": out.SimRuns,
		})
	}
}

// AdminSessionCommands opens the shell history of one drill.
//
// The most sensitive read on the platform: a verbatim record of what somebody
// typed, including whatever they mistyped a password into. Its own endpoint, its
// own audit entry, and never folded into a page that loads by itself — so
// opening one is always a decision somebody made and can be asked about.
func (h *Handler) AdminSessionCommands(c *gin.Context) {
	id := c.Param("id")
	log, err := h.uc.SessionCommands(c.Request.Context(), id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case err != nil:
		serverError(c, err)
	default:
		h.audit.Record(c, audit.Entry{
			ActorID:    userID(c),
			Action:     audit.ActionCommandLogRead,
			TargetType: audit.TargetSession,
			TargetID:   id,
		})
		c.JSON(http.StatusOK, gin.H{"command_log": log})
	}
}
