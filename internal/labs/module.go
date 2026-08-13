package labs

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/devforge/be/internal/audit"
	"github.com/devforge/be/internal/events"
	"github.com/devforge/be/internal/labs/adapter/dockerx"
	"github.com/devforge/be/internal/labs/adapter/openrouter"
	"github.com/devforge/be/internal/labs/adapter/quota"
	"github.com/devforge/be/internal/labs/adapter/ratelimit"
	"github.com/devforge/be/internal/labs/adapter/repo"
	"github.com/devforge/be/internal/labs/adapter/rest"
	"github.com/devforge/be/internal/labs/usecase"
	"github.com/devforge/be/internal/labs/usecase/simgen"
)

type Config struct {
	DockerHost     string
	SessionTTL     time.Duration
	AllowedOrigins []string
	// Either empty switches off scenario generation. The server still starts and
	// the endpoint answers 503: one optional feature must not be a boot
	// requirement.
	OpenRouterKey   string
	OpenRouterModel string
	// Sent to OpenRouter for traffic attribution, and the origin the preview
	// card's image URL is built from — a crawler needs an absolute one.
	PublicURL string
	// Where the browser-facing app lives. The preview page redirects a human to
	// it, and tells crawlers this is the canonical address of the report.
	FrontendURL string
	// Generations allowed per user per day. Every one of them is a paid call, so
	// this is a cost ceiling, not a fairness knob.
	AIDailyLimit int
	// Requests a minute, per address, on the routes that answer without an
	// account. Zero switches the cap off — which is what the tests and a local
	// run want, and what production must never have.
	PublicRateLimit int
	// Lab containers allowed on the host at once. Zero takes the default.
	MaxContainers int
}

type Module struct {
	handler *rest.Handler
	// Guards the two routes with no caller behind them. Nil-safe: switched off,
	// its middleware is a pass-through.
	publicLimit *ratelimit.Redis
	terminal    *rest.Terminal
	// The simulator offered as a tool rather than as a lesson. It lives in this
	// module because it runs the same engine, and nowhere near the rest of it
	// because it touches no session, no container and no mark.
	playground *rest.Playground
	// Writing a scenario from a sentence. Sits here rather than in its own
	// module because what makes it safe is this module's engine: everything the
	// model produces is checked and run by `sim` before it leaves the server.
	simgen  *rest.Simgen
	uc      *usecase.Labs
	runtime *dockerx.Runtime
}

// New fails rather than degrades when the socket proxy is unreachable. A server
// that boots without a runtime looks healthy right up until the first student
// presses Start.
func New(ctx context.Context, db *gorm.DB, rdb *redis.Client, cfg Config) (*Module, error) {
	rt, err := dockerx.New(cfg.DockerHost)
	if err != nil {
		return nil, err
	}
	if err := rt.Ping(ctx); err != nil {
		return nil, err
	}

	uc := usecase.NewLabs(
		repo.NewSessionRepo(db), repo.NewGradeRepo(db), repo.NewSimRepo(db),
		rt, cfg.SessionTTL, cfg.MaxContainers,
	)
	return &Module{
		publicLimit: ratelimit.New(rdb, cfg.PublicRateLimit, time.Minute, "rl:public"),
		handler:     rest.NewHandler(uc, cfg.PublicURL, cfg.FrontendURL),
		terminal:    rest.NewTerminal(uc, cfg.AllowedOrigins),
		playground:  rest.NewPlayground(usecase.NewPlayground()),
		simgen: rest.NewSimgen(simgen.New(
			openrouter.New(cfg.OpenRouterKey, cfg.OpenRouterModel, cfg.PublicURL),
			quota.New(rdb, cfg.AIDailyLimit),
		)),
		uc:      uc,
		runtime: rt,
	}, nil
}

// SetAudit passes the recorder through to the handler, for the one endpoint
// that ends a session belonging to somebody else.
func (m *Module) SetAudit(a *audit.Recorder) { m.handler.SetAudit(a) }

// SetEvents wires the system-event recorder into everything in this module that
// can fail in a way an admin would want to see: starting a lab, running a check
// script, breaking a service, reaping a container, and the cap on the public
// routes.
func (m *Module) SetEvents(r *events.Recorder) {
	m.uc.SetEvents(r)
	m.publicLimit.SetEvents(r)
	m.handler.SetEvents(r)
}

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

	// Màn quản trị đọc. Tất cả admin-only, tất cả chỉ đọc.
	//
	// Hai cái cuối có ghi audit: đọc hoạt động của một người và mở lịch sử lệnh
	// của họ là **đọc việc của người khác**, không phải đọc một con số của hệ
	// thống — nên ai mở cũng để lại tên.
	api.GET("/admin/overview", required, admin, h.AdminOverview)
	api.GET("/admin/events", required, admin, h.AdminEvents)
	api.GET("/admin/content-health", required, admin, h.AdminContentHealth)
	api.GET("/admin/shared-drills", required, admin, h.AdminSharedReports)
	api.GET("/admin/users/:id/activity", required, admin, h.AdminUserActivity)
	api.GET("/admin/lab-sessions/:id/commands", required, admin, h.AdminSessionCommands)

	// Killing someone else's container. Separate path from the student's own
	// DELETE /lab-sessions/:id rather than a role branch inside it: that one
	// answers 404 for a session the caller does not own, and weakening it to
	// let admins through would weaken it for everyone.
	// Gỡ một báo cáo công khai. Nhận token vì đó là thứ người báo cáo gửi tới:
	// một cái link, không phải một id phiên.
	api.DELETE("/admin/shared-drills/:token", required, admin, h.AdminUnshareDrill)

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
	// Publishing a drill report, and taking it back down. Behind the owner's own
	// session like every route above it: what goes public is decided by the
	// person it describes.
	api.POST("/lab-sessions/:id/share", required, h.Share)
	api.DELETE("/lab-sessions/:id/share", required, h.Unshare)
	// Grading is scoped to a session because a check script only means anything
	// against the container that session owns.
	api.POST("/lab-sessions/:id/tasks/:taskID/check", required, h.Check)

	// A sim lab runs its pipeline through the same session that would otherwise
	// hold a container, so checking and submitting stay on the routes above. Only
	// pressing Run is new, and reading back what was run.
	api.POST("/lab-sessions/:id/sim/run", required, h.SimRun)
	api.GET("/lab-sessions/:id/sim/runs", required, h.SimRuns)

	// The playground: the simulator with no lab, no session, no mark — and no
	// database, because its scenarios live in the frontend's own source. Signed
	// in, but nothing beyond that: no enrolment, no container, nothing stored.
	api.POST("/sim/preview", required, m.playground.Preview)

	// Describing a pipeline in a sentence and getting a scenario back. Same
	// place in the tree as Preview because it is the same tool: it produces
	// something the playground runs, and nothing a lab is graded on.
	api.POST("/sim/generate", required, m.simgen.Generate)

	// The two public reads, with no `required` in front of them.
	//
	// A shared report has to open for somebody who has never signed in — that is
	// the whole point of a link you can paste — and the day's challenge sits on
	// the same page, so asking a stranger to sign in just to see what today's
	// scenario is loses them at the door. Neither one can be steered: one takes a
	// token that has to have been minted, the other takes nothing at all.
	//
	// `/shared-drills` and `/daily-drill` rather than hanging off `/war-room`,
	// which the courses module owns and where `:slug` already sits — a static
	// segment beside an existing wildcard is a route conflict waiting for a
	// deploy to find it.
	limit := m.publicLimit.Middleware()
	api.GET("/shared-drills/:token", limit, h.SharedDrill)
	api.GET("/daily-drill", limit, h.Daily)
	api.GET("/weekly-board", limit, h.Weekly)
	// Chuỗi ngày của chính người gọi — thứ duy nhất trong nhóm này chỉ có nghĩa
	// với một người, nên nó nằm sau đăng nhập.
	api.GET("/my-drill-streak", required, h.Streak)
	// The two a crawler reads. Same token, same 404 once it is taken down, so an
	// unpublished report stops having a picture as well as a page.
	api.GET("/shared-drills/:token/preview", limit, h.SharedDrillPreview)
	api.GET("/shared-drills/:token/og.png", limit, h.SharedDrillOG)

	// Outside /api because it is not one: the client opens it with a WebSocket
	// handshake, and the cookie the middleware reads rides along with it.
	r.GET("/ws/terminal/:id", required, m.terminal.Handle)
}
