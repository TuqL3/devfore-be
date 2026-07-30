package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Env      string `env:"APP_ENV" envDefault:"development"`
	Port     string `env:"PORT" envDefault:"8080"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	DBHost string `env:"DB_HOST,required"`
	DBPort string `env:"DB_PORT" envDefault:"5432"`
	DBUser string `env:"DB_USER,required"`
	DBPass string `env:"DB_PASSWORD,required"`
	DBName string `env:"DB_NAME,required"`

	RedisAddr string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	RedisPass string `env:"REDIS_PASSWORD"`
	RedisDB   int    `env:"REDIS_DB" envDefault:"0"`

	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:","`

	CookieDomain string `env:"COOKIE_DOMAIN"`

	JWTSecret          string        `env:"JWT_SECRET,required"`
	AccessTTL          time.Duration `env:"ACCESS_TTL" envDefault:"15m"`
	RefreshTTL         time.Duration `env:"REFRESH_TTL" envDefault:"168h"`
	GoogleClientID     string        `env:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret string        `env:"GOOGLE_CLIENT_SECRET"`
	GoogleRedirectURL  string        `env:"GOOGLE_REDIRECT_URL" envDefault:"http://localhost:8080/api/auth/google/callback"`
	FrontendURL        string        `env:"FRONTEND_URL" envDefault:"http://localhost:5173"`

	// Leave SMTP_HOST empty in development: the mailer then prints the code and
	// the reset link to the server log instead of sending them.
	SMTPHost string `env:"SMTP_HOST"`
	SMTPPort string `env:"SMTP_PORT" envDefault:"587"`
	SMTPUser string `env:"SMTP_USER"`
	SMTPPass string `env:"SMTP_PASSWORD"`
	MailFrom string `env:"MAIL_FROM" envDefault:"no-reply@devforge.local"`

	VerifyCodeTTL  time.Duration `env:"VERIFY_CODE_TTL" envDefault:"10m"`
	ResetTokenTTL  time.Duration `env:"RESET_TOKEN_TTL" envDefault:"1h"`
	ResendCooldown time.Duration `env:"RESEND_COOLDOWN" envDefault:"60s"`

	UploadDir string `env:"UPLOAD_DIR" envDefault:"./uploads"`
	PublicURL string `env:"PUBLIC_URL" envDefault:"http://localhost:8080"`

	// Points at docker-socket-proxy, never at /var/run/docker.sock. The proxy is
	// what keeps a bug in the lab code from reaching the rest of the daemon.
	// Not named DOCKER_HOST: that one is read by the docker CLI too, so putting
	// it in .env would quietly route `docker compose` through the proxy and have
	// it denied halfway through.
	DockerHost string `env:"LAB_DOCKER_HOST" envDefault:"tcp://127.0.0.1:2375"`
	// How long a lab container may live. The reaper reads the deadline this
	// produces out of the database, not out of a timer.
	LabSessionTTL time.Duration `env:"LAB_SESSION_TTL" envDefault:"60m"`
}

func Load() (*Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	return &c, nil
}

func (c *Config) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		c.DBHost, c.DBPort, c.DBUser, c.DBPass, c.DBName)
}

func (c *Config) IsProd() bool { return c.Env == "production" }
