package auth

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/devforge/be/internal/auth/adapter/hash"
	"github.com/devforge/be/internal/auth/adapter/oauthgoogle"
	"github.com/devforge/be/internal/auth/adapter/repo"
	"github.com/devforge/be/internal/auth/adapter/rest"
	"github.com/devforge/be/internal/auth/adapter/token"
	"github.com/devforge/be/internal/auth/usecase"
)

type Config struct {
	JWTSecret          string
	AccessTTL          time.Duration
	RefreshTTL         time.Duration
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	FrontendURL        string
	UploadDir          string
	PublicURL          string
}

type Module struct {
	handler *rest.Handler
	mw      *rest.Middleware
}

func New(db *gorm.DB, cfg Config) *Module {
	users := repo.NewUserRepo(db)
	tokens := token.NewJWT(cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)
	hasher := hash.Bcrypt{}
	google := oauthgoogle.New(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL)

	uc := usecase.NewAuth(users, tokens, hasher, google)

	return &Module{
		handler: rest.NewHandler(uc, cfg.FrontendURL, cfg.UploadDir, cfg.PublicURL),
		mw:      rest.NewMiddleware(uc),
	}
}

func (m *Module) Required() gin.HandlerFunc { return m.mw.Required() }
func (m *Module) Optional() gin.HandlerFunc { return m.mw.Optional() }

func (m *Module) Routes(api *gin.RouterGroup) {
	h := m.handler
	g := api.Group("/auth")
	g.POST("/register", h.Register)
	g.POST("/login", h.Login)
	g.POST("/refresh", h.Refresh)
	g.GET("/google", h.GoogleLogin)
	g.GET("/google/callback", h.GoogleCallback)

	api.GET("/me", m.mw.Required(), h.Me)
	api.PATCH("/me", m.mw.Required(), h.UpdateMe)
	api.PATCH("/me/password", m.mw.Required(), h.ChangePassword)
	api.DELETE("/me", m.mw.Required(), h.DeleteMe)
	api.POST("/me/avatar", m.mw.Required(), h.UploadAvatar)
}
