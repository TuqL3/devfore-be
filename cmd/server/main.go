package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/auth"
	"github.com/devforge/be/internal/config"
	"github.com/devforge/be/internal/courses"
	"github.com/devforge/be/internal/db"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(cfg.LogLevel)})))

	gdb, err := db.Open(cfg.DSN(), cfg.IsProd())
	if err != nil {
		return err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	rdb, err := db.OpenRedis(cfg.RedisAddr, cfg.RedisPass, cfg.RedisDB)
	if err != nil {
		return err
	}
	defer rdb.Close()

	authMod := auth.New(gdb, rdb, auth.Config{
		JWTSecret:          cfg.JWTSecret,
		AccessTTL:          cfg.AccessTTL,
		RefreshTTL:         cfg.RefreshTTL,
		GoogleClientID:     cfg.GoogleClientID,
		GoogleClientSecret: cfg.GoogleClientSecret,
		GoogleRedirectURL:  cfg.GoogleRedirectURL,
		FrontendURL:        cfg.FrontendURL,
		UploadDir:          cfg.UploadDir,
		PublicURL:          cfg.PublicURL,
		CookieDomain:       cfg.CookieDomain,
		// Only ever sent over TLS in production. Locally there is no TLS, and a
		// Secure cookie would simply never be stored.
		CookieSecure: cfg.IsProd(),
	})
	coursesMod := courses.New(gdb)

	if cfg.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	// Gin trusts every proxy by default, which lets any client set its own
	// X-Forwarded-For and choose the IP shown on the devices screen. In prod the
	// only real proxy is Caddy on this same host.
	if err := r.SetTrustedProxies([]string{"127.0.0.1", "::1"}); err != nil {
		return err
	}
	r.Use(gin.Recovery(), requestLog(), cors(cfg.CORSOrigins))

	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db unreachable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
	r.Static("/uploads", cfg.UploadDir)

	api := r.Group("/api")
	api.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "pong"}) })
	authMod.Routes(api)
	coursesMod.Routes(api, authMod.Required(), authMod.Optional())

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http listening", "addr", srv.Addr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func logLevel(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return l
}

func requestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"took", time.Since(start).String())
	}
}

// Auth rides on cookies, which the browser attaches to cross-site requests all
// by itself. Two things keep that from becoming CSRF: SameSite=Lax on the
// cookies themselves, and this — a hard refusal to act on a state-changing
// request from an origin that is not ours.
//
// Requests with no Origin header pass: browsers always send one on the methods
// that matter here, so the header-less case is a CLI, not a forged form.
func cors(allowed []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		ok := origin != "" && (slices.Contains(allowed, origin) || sameOrigin(origin, c.Request.Host))

		if ok {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type")
			// Without this the browser drops the cookies from the response of a
			// cross-origin call, and login silently does nothing.
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Max-Age", "600")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		if origin != "" && !ok && !safeMethod(c.Request.Method) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "cross-origin request rejected"})
			return
		}
		c.Next()
	}
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// In production the frontend is served from the same host as the API, so the
// allowlist is usually empty and this is what lets the app talk to itself.
func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Host == host
}
