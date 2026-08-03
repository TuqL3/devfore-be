package chat

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

type Config struct {
	AllowedOrigins []string
	// How long one connection may stay open before the client has to reconnect
	// and be authorised again. Set from the access token's lifetime — see
	// Handle for why the two are tied together.
	SessionWindow time.Duration
}

type Module struct {
	db            *gorm.DB
	repo          *Repo
	hub           *Hub
	upgrader      websocket.Upgrader
	sessionWindow time.Duration
}

func New(db *gorm.DB, cfg Config) *Module {
	window := cfg.SessionWindow
	if window <= 0 {
		// A zero here would close every connection immediately, which reads as
		// "chat is broken" rather than "config is missing".
		window = 15 * time.Minute
	}
	return &Module{
		db:   db,
		repo: NewRepo(db),
		hub:  NewHub(),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  2048,
			WriteBufferSize: 2048,
			CheckOrigin:     originAllowed(cfg.AllowedOrigins),
		},
		sessionWindow: window,
	}
}

// Routes mounts the room. Both endpoints are behind the caller's Required
// middleware: the room is for signed-in students, and the websocket handshake
// is authorised by the same cookie as everything else.
func (m *Module) Routes(r *gin.Engine, api *gin.RouterGroup, required gin.HandlerFunc) {
	api.GET("/chat/messages", required, m.Messages)
	api.GET("/chat/conversations", required, m.Conversations)
	api.GET("/chat/people", required, m.People)
	// Outside the /api group's json handlers for the same reason the lab
	// terminal is: an upgrade is not a json response.
	r.GET("/ws/chat", required, m.Handle)
}

// userID reads what the auth middleware left on the context. Read here rather
// than imported from the auth adapter, which is the convention the labs module
// already follows — the key is the contract between them, not a Go symbol.
func (m *Module) userID(c *gin.Context) int64 {
	v, _ := c.Get("user_id")
	id, _ := v.(int64)
	return id
}

// username resolves the display name once per connection. One query on connect
// rather than a field threaded through the access token: the token is issued
// before a rename and would keep showing the old name until it expired.
func (m *Module) username(c *gin.Context) string {
	id := m.userID(c)
	if id == 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	var name string
	if err := m.db.WithContext(ctx).Raw(
		`SELECT username FROM users WHERE id = ? AND status = 'active'`, id,
	).Scan(&name).Error; err != nil {
		return ""
	}
	// Empty when the account is banned or gone. Both mean no room for them, and
	// the caller turns that into a 401.
	return name
}
