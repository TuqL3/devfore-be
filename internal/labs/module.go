package labs

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/devforge/be/internal/audit"
	"github.com/devforge/be/internal/labs/adapter/dockerx"
	"github.com/devforge/be/internal/labs/adapter/repo"
	"github.com/devforge/be/internal/labs/adapter/rest"
	"github.com/devforge/be/internal/labs/usecase"
)

type Config struct {
	DockerHost     string
	SessionTTL     time.Duration
	AllowedOrigins []string
}

type Module struct {
	handler  *rest.Handler
	terminal *rest.Terminal
	uc       *usecase.Labs
	runtime  *dockerx.Runtime
}

// New fails rather than degrades when the socket proxy is unreachable. A server
// that boots without a runtime looks healthy right up until the first student
// presses Start.
func New(ctx context.Context, db *gorm.DB, cfg Config) (*Module, error) {
	rt, err := dockerx.New(cfg.DockerHost)
	if err != nil {
		return nil, err
	}
	if err := rt.Ping(ctx); err != nil {
		return nil, err
	}

	uc := usecase.NewLabs(repo.NewSessionRepo(db), repo.NewGradeRepo(db), rt, cfg.SessionTTL)
	return &Module{
		handler:  rest.NewHandler(uc),
		terminal: rest.NewTerminal(uc, cfg.AllowedOrigins),
		uc:       uc,
		runtime:  rt,
	}, nil
}

// SetAudit passes the recorder through to the handler, for the one endpoint
// that ends a session belonging to somebody else.
func (m *Module) SetAudit(a *audit.Recorder) { m.handler.SetAudit(a) }

// StartReaper runs the sweep until ctx is cancelled. Kept separate from New so
// the caller decides its lifetime alongside the server's.
func (m *Module) StartReaper(ctx context.Context) { go m.uc.Reap(ctx) }

func (m *Module) Close() error { return m.runtime.Close() }

func (m *Module) Routes(r *gin.Engine, api *gin.RouterGroup, required, admin gin.HandlerFunc) {
	h := m.handler

	// Running a script an author just typed needs a container, and containers
	// live in this module. Admin-only: it executes shell the caller supplies.
	api.POST("/admin/check-scripts/try", required, admin, h.TryScript)

	// The overview counts sessions and answers, which live here. Courses and
	// users are counted alongside them rather than from their own modules: one
	// screen asking one endpoint beats three round trips the client has to
	// stitch together.
	api.GET("/admin/stats", required, admin, h.AdminStats)

	// Killing someone else's container. Separate path from the student's own
	// DELETE /lab-sessions/:id rather than a role branch inside it: that one
	// answers 404 for a session the caller does not own, and weakening it to
	// let admins through would weaken it for everyone.
	api.GET("/admin/lab-sessions", required, admin, h.AdminRunningSessions)
	api.DELETE("/admin/lab-sessions/:id", required, admin, h.AdminKillSession)

	api.POST("/labs/:slug/start", required, h.Start)
	api.GET("/lab-sessions/current", required, h.Current)
	// Static before param, or /lab-sessions/history reads as a session id.
	api.GET("/lab-sessions/history", required, h.History)
	api.GET("/lab-sessions/:id", required, h.Session)
	api.DELETE("/lab-sessions/:id", required, h.Stop)
	// Handing in ends the session and freezes its report; the report itself is
	// readable afterwards, however the session ended.
	api.POST("/lab-sessions/:id/submit", required, h.Submit)
	api.GET("/lab-sessions/:id/report", required, h.Report)
	// Grading is scoped to a session because a check script only means anything
	// against the container that session owns.
	api.POST("/lab-sessions/:id/tasks/:taskID/check", required, h.Check)

	// Outside /api because it is not one: the client opens it with a WebSocket
	// handshake, and the cookie the middleware reads rides along with it.
	r.GET("/ws/terminal/:id", required, m.terminal.Handle)
}
