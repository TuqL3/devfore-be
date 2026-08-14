package rest

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/audit"
	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase"
)

// sharedDrill is the public payload, and the list of fields is the feature's
// security boundary as much as the query behind it is. No timeline, no
// walkthrough, no email, no session id — see domain.SharedDrill for why each one
// is absent.
type sharedDrill struct {
	Token    string `json:"token"`
	LabSlug  string `json:"lab_slug"`
	LabTitle string `json:"lab_title"`
	// The scenario to hand a reader who takes the challenge. The whole of "the
	// same seed": a drill replays a row, not a random number.
	IncidentID int64 `json:"incident_id"`
	// Names the fault, so it is a spoiler. Sent anyway — the result is about this
	// fault — and the page keeps it behind a deliberate click.
	IncidentTitle   string    `json:"incident_title"`
	Player          string    `json:"player"`
	StartedAt       time.Time `json:"started_at"`
	Recovered       bool      `json:"recovered"`
	DowntimeSeconds int       `json:"downtime_seconds"`
	RequestsFailed  int       `json:"requests_failed"`
	// Assumed requests per second, sent so the page can say the count is derived
	// from an authored figure rather than measured.
	RPS int `json:"rps"`
}

type drillLeader struct {
	Player          string `json:"player"`
	DowntimeSeconds int    `json:"downtime_seconds"`
	RequestsFailed  int    `json:"requests_failed"`
}

type dailyDrill struct {
	Day        string        `json:"day"`
	LabSlug    string        `json:"lab_slug"`
	LabTitle   string        `json:"lab_title"`
	IncidentID int64         `json:"incident_id"`
	Leaders    []drillLeader `json:"leaders"`
}

// Share publishes the caller's own finished drill. Answers the token rather than
// a full URL: the front end knows its own origin, and a server guessing at one
// gets it wrong the first time the site is opened through a different host.
func (h *Handler) Share(c *gin.Context) {
	token, err := h.uc.Share(c.Request.Context(), c.Param("id"), userID(c))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case errors.Is(err, domain.ErrNotDrill):
		abort(c, http.StatusBadRequest, "chỉ ca trực mới có báo cáo chia sẻ được")
	case errors.Is(err, domain.ErrStillRunning):
		abort(c, http.StatusConflict, "phiên này chưa kết thúc")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, gin.H{"token": token})
	}
}

func (h *Handler) Unshare(c *gin.Context) {
	err := h.uc.Unshare(c.Request.Context(), c.Param("id"), userID(c))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case err != nil:
		serverError(c, err)
	default:
		c.Status(http.StatusNoContent)
	}
}

// AdminUnshareDrill removes a published report that should not be public.
//
// Takes the token, which is what a moderator has: somebody forwards them a
// link. Audited like every other admin action on another person's work.
func (h *Handler) AdminUnshareDrill(c *gin.Context) {
	token := c.Param("token")
	sessionID, err := h.uc.AdminUnshare(c.Request.Context(), token)
	if err == nil {
		h.audit.Record(c, audit.Entry{
			ActorID:    userID(c),
			Action:     audit.ActionDrillUnshare,
			TargetType: audit.TargetSession,
			TargetID:   sessionID,
		})
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		// Already down, or never up. Both are the outcome the caller wanted.
		abort(c, http.StatusNotFound, "báo cáo này không còn được chia sẻ")
	case err != nil:
		serverError(c, err)
	default:
		c.Status(http.StatusNoContent)
	}
}

// SharedDrill is the only route on the platform that answers without a caller.
// A token that was never minted and a token that has been taken down give the
// same 404 — whether a link ever existed is not something to confirm.
func (h *Handler) SharedDrill(c *gin.Context) {
	d, err := h.uc.Shared(c.Request.Context(), c.Param("token"))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "báo cáo này không còn được chia sẻ")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, sharedDrill{
			Token:           d.Token,
			LabSlug:         d.LabSlug,
			LabTitle:        d.LabTitle,
			IncidentID:      d.IncidentID,
			IncidentTitle:   d.IncidentTitle,
			Player:          d.Player,
			StartedAt:       d.StartedAt,
			Recovered:       d.Recovered,
			DowntimeSeconds: d.DowntimeSeconds,
			RequestsFailed:  d.RequestsFailed,
			RPS:             d.RPS,
		})
	}
}

type weeklyLeader struct {
	Player     string `json:"player"`
	DaysSolved int    `json:"days_solved"`
	TotalTime  int    `json:"total_time"`
}

// Weekly is the seven-day board. Public, like the day's own.
//
// Days solved rather than seconds: seven days is seven different faults, and a
// sum across them would rank the week somebody drew as much as how they did.
func (h *Handler) Weekly(c *gin.Context) {
	rows, err := h.uc.Weekly(c.Request.Context(), time.Now())
	switch {
	case errors.Is(err, domain.ErrNoDailyDrill):
		abort(c, http.StatusNotFound, "chưa có ca trực nào được xuất bản")
	case err != nil:
		serverError(c, err)
	default:
		out := make([]weeklyLeader, len(rows))
		for i, r := range rows {
			out[i] = weeklyLeader{Player: r.Player, DaysSolved: r.DaysSolved, TotalTime: r.TotalTime}
		}
		c.JSON(http.StatusOK, gin.H{"days": usecase.WeeklyDays, "leaders": out})
	}
}

// Streak is the caller's own run of days. Behind sign-in, unlike the boards:
// it is about one person, and it is the one number on this feature that only
// means something to them.
func (h *Handler) Streak(c *gin.Context) {
	st, err := h.uc.Streak(c.Request.Context(), userID(c), time.Now())
	switch {
	case errors.Is(err, domain.ErrNoDailyDrill):
		// No scenarios published: no streak to have, and not an error.
		c.JSON(http.StatusOK, gin.H{"current": 0, "longest": 0, "solved_today": false})
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, gin.H{
			"current":      st.Current,
			"longest":      st.Longest,
			"solved_today": st.SolvedToday,
		})
	}
}

// Daily answers today's scenario and its board, to anybody. Public on purpose:
// it is the page a shared link lands next to, and asking a stranger to sign in
// before they can see what today's challenge even is loses them there.
//
// A platform with no published drill answers 404 rather than an empty object —
// there is no day to describe, and a client rendering an empty board would be
// showing a challenge that does not exist.
func (h *Handler) Daily(c *gin.Context) {
	now := time.Now()

	// `?day=` reads an archived board instead of today's. One call or the other,
	// never both: asking for today and then throwing it away would double the
	// queries on the one route a crawler walks day by day.
	var (
		d   *domain.DailyDrill
		err error
	)
	if raw := c.Query("day"); raw != "" {
		day, perr := time.ParseInLocation(time.DateOnly, raw, time.UTC)
		if perr != nil {
			abort(c, http.StatusBadRequest, "ngày không hợp lệ")
			return
		}
		d, err = h.uc.Day(c.Request.Context(), day, now)
	} else {
		d, err = h.uc.Daily(c.Request.Context(), now)
	}

	switch {
	case errors.Is(err, domain.ErrNoDailyDrill):
		// Also the answer for a day nobody can have: tomorrow, or further back
		// than the archive goes.
		abort(c, http.StatusNotFound, "chưa có ca trực nào được xuất bản")
	case err != nil:
		serverError(c, err)
	default:
		leaders := make([]drillLeader, len(d.Leaders))
		for i, l := range d.Leaders {
			leaders[i] = drillLeader{
				Player:          l.Player,
				DowntimeSeconds: l.DowntimeSeconds,
				RequestsFailed:  l.RequestsFailed,
			}
		}
		c.JSON(http.StatusOK, dailyDrill{
			Day:        d.Day,
			LabSlug:    d.LabSlug,
			LabTitle:   d.LabTitle,
			IncidentID: d.IncidentID,
			Leaders:    leaders,
		})
	}
}
