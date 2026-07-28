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

	// Refresh sessions live here, so the API cannot serve logins without it.
	RedisAddr string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	RedisPass string `env:"REDIS_PASSWORD"`
	RedisDB   int    `env:"REDIS_DB" envDefault:"0"`

	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:","`

	// Only needed when the cookie has to span subdomains; empty means
	// host-only, which is the safer default.
	CookieDomain string `env:"COOKIE_DOMAIN"`

	JWTSecret          string        `env:"JWT_SECRET,required"`
	AccessTTL          time.Duration `env:"ACCESS_TTL" envDefault:"15m"`
	RefreshTTL         time.Duration `env:"REFRESH_TTL" envDefault:"168h"`
	GoogleClientID     string        `env:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret string        `env:"GOOGLE_CLIENT_SECRET"`
	GoogleRedirectURL  string        `env:"GOOGLE_REDIRECT_URL" envDefault:"http://localhost:8080/api/auth/google/callback"`
	FrontendURL        string        `env:"FRONTEND_URL" envDefault:"http://localhost:5173"`

	UploadDir string `env:"UPLOAD_DIR" envDefault:"./uploads"`
	// Absolute base the browser uses to fetch uploaded files.
	PublicURL string `env:"PUBLIC_URL" envDefault:"http://localhost:8080"`
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
