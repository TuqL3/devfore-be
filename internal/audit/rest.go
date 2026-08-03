package audit

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type logRow struct {
	ID         int64     `json:"id"`
	ActorName  string    `json:"actor_name"`
	Action     string    `json:"action"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	TargetName string    `json:"target_name"`
	Detail     string    `json:"detail"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
}

type listResponse struct {
	Logs  []logRow `json:"logs"`
	Total int      `json:"total"`
	Limit int      `json:"limit"`
}

// Routes mounts the read side. There is no write endpoint: entries are written
// by the code performing the action, and an API that let a client post its own
// audit trail would make the whole table worthless.
func (r *Recorder) Routes(api *gin.RouterGroup, required, admin gin.HandlerFunc) {
	api.GET("/admin/audit-logs", required, admin, r.list)
}

func (r *Recorder) list(c *gin.Context) {
	page, err := r.List(c.Request.Context(), Filter{
		Action: c.Query("action"),
		Actor:  c.Query("actor"),
	})
	if err != nil {
		slog.Error("audit list", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "lỗi máy chủ"})
		return
	}
	rows := make([]logRow, len(page.Logs))
	for i, l := range page.Logs {
		rows[i] = logRow{
			ID:         l.ID,
			ActorName:  l.ActorName,
			Action:     l.Action,
			TargetType: l.TargetType,
			TargetID:   l.TargetID,
			TargetName: l.TargetName,
			Detail:     l.Detail,
			IP:         l.IP,
			CreatedAt:  l.CreatedAt,
		}
	}
	c.JSON(http.StatusOK, listResponse{Logs: rows, Total: page.Total, Limit: page.Limit})
}

// Record writes an entry and swallows the error into the log. The action it
// describes has already happened by the time this runs; failing the request now
// would tell the caller nothing happened, which is the one thing that is not
// true. Every call site uses this rather than Write for that reason.
func (r *Recorder) Record(c *gin.Context, e Entry) {
	// A build that forgot to wire the recorder would otherwise panic on the
	// first moderation request. Shouting into the log is the middle ground: the
	// action still goes through, and the gap is visible rather than silent.
	if r == nil {
		slog.Error("audit recorder not wired", "action", e.Action, "target", e.TargetID)
		return
	}
	e.IP = c.ClientIP()
	if err := r.Write(c.Request.Context(), e); err != nil {
		slog.Error("audit write failed",
			"action", e.Action, "actor", e.ActorID, "target", e.TargetID, "err", err)
	}
}
