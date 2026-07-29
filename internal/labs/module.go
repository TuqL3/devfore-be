package labs

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

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

	uc := usecase.NewLabs(repo.NewSessionRepo(db), rt, cfg.SessionTTL)
	return &Module{
		handler:  rest.NewHandler(uc),
		terminal: rest.NewTerminal(uc, cfg.AllowedOrigins),
		uc:       uc,
		runtime:  rt,
	}, nil
}

// StartReaper runs the sweep until ctx is cancelled. Kept separate from New so
// the caller decides its lifetime alongside the server's.
func (m *Module) StartReaper(ctx context.Context) { go m.uc.Reap(ctx) }

func (m *Module) Close() error { return m.runtime.Close() }

func (m *Module) Routes(r *gin.Engine, api *gin.RouterGroup, required gin.HandlerFunc) {
	h := m.handler

	api.POST("/labs/:slug/start", required, h.Start)
	api.GET("/lab-sessions/current", required, h.Current)
	api.GET("/lab-sessions/:id", required, h.Session)
	api.DELETE("/lab-sessions/:id", required, h.Stop)

	// Outside /api because it is not one: the client opens it with a WebSocket
	// handshake, and the cookie the middleware reads rides along with it.
	r.GET("/ws/terminal/:id", required, m.terminal.Handle)
}
