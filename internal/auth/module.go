package auth

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/devforge/be/internal/auth/adapter/hash"
	"github.com/devforge/be/internal/auth/adapter/mail"
	"github.com/devforge/be/internal/auth/adapter/oauthgoogle"
	"github.com/devforge/be/internal/auth/adapter/repo"
	"github.com/devforge/be/internal/auth/adapter/rest"
	"github.com/devforge/be/internal/auth/adapter/session"
	"github.com/devforge/be/internal/auth/adapter/token"
	"github.com/devforge/be/internal/auth/adapter/verify"
	"github.com/devforge/be/internal/auth/domain"
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
	CookieDomain       string
	CookieSecure       bool

	SMTPHost       string
	SMTPPort       string
	SMTPUser       string
	SMTPPass       string
	MailFrom       string
	VerifyCodeTTL  time.Duration
	ResetTokenTTL  time.Duration
	ResendCooldown time.Duration
}

type Module struct {
	handler *rest.Handler
	mw      *rest.Middleware
}

func New(db *gorm.DB, rdb *redis.Client, cfg Config) *Module {
	users := repo.NewUserRepo(db)
	tokens := token.NewJWT(cfg.JWTSecret, cfg.AccessTTL)
	sessions := session.New(rdb, cfg.RefreshTTL)
	hasher := hash.Bcrypt{}
	google := oauthgoogle.New(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL)

	codes := verify.New(rdb)

	var mailer usecase.Mailer = mail.Log{}
	if cfg.SMTPHost != "" {
		mailer = mail.NewSMTP(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.MailFrom)
	}

	uc := usecase.NewAuth(users, tokens, hasher, google, sessions, codes, mailer, usecase.Verification{
		CodeTTL:  cfg.VerifyCodeTTL,
		ResetTTL: cfg.ResetTokenTTL,
		Resend:   cfg.ResendCooldown,
		AppURL:   cfg.FrontendURL,
	})
	cookies := rest.CookieConfig{Domain: cfg.CookieDomain, Secure: cfg.CookieSecure}

	return &Module{
		handler: rest.NewHandler(uc, cfg.FrontendURL, cfg.UploadDir, cfg.PublicURL, cookies),
		mw:      rest.NewMiddleware(uc),
	}
}

func (m *Module) Required() gin.HandlerFunc { return m.mw.Required() }
func (m *Module) Optional() gin.HandlerFunc { return m.mw.Optional() }

// AdminOnly has to run behind Required: it reads the roles that one puts on the
// context, and by itself would see an anonymous request as a user with none.
func (m *Module) AdminOnly() gin.HandlerFunc { return rest.RequireRole(domain.RoleAdmin) }

func (m *Module) Routes(api *gin.RouterGroup) {
	h := m.handler
	g := api.Group("/auth")
	g.POST("/register", h.Register)
	g.POST("/verify-email", h.VerifyEmail)
	g.POST("/resend-code", h.ResendCode)
	g.POST("/forgot-password", h.ForgotPassword)
	g.POST("/reset-password", h.ResetPassword)
	g.POST("/login", h.Login)
	g.POST("/refresh", h.Refresh)
	g.POST("/logout", h.Logout)
	g.POST("/logout-all", m.mw.Required(), h.LogoutAll)
	g.GET("/sessions", m.mw.Required(), h.Sessions)
	g.DELETE("/sessions/:id", m.mw.Required(), h.RevokeSession)
	g.GET("/google", h.GoogleLogin)
	g.GET("/google/callback", h.GoogleCallback)

	api.GET("/me", m.mw.Required(), h.Me)
	api.PATCH("/me", m.mw.Required(), h.UpdateMe)
	api.PATCH("/me/password", m.mw.Required(), h.ChangePassword)
	api.DELETE("/me", m.mw.Required(), h.DeleteMe)
	api.POST("/me/avatar", m.mw.Required(), h.UploadAvatar)
}
