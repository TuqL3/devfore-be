package config

import (
	"fmt"

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

	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:","`
}

func Load() (*Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	return &c, nil
}

// DSN returns the Postgres connection string.
func (c *Config) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		c.DBHost, c.DBPort, c.DBUser, c.DBPass, c.DBName)
}

func (c *Config) IsProd() bool { return c.Env == "production" }
