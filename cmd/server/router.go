package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/audit"
	"github.com/devforge/be/internal/auth"
	"github.com/devforge/be/internal/chat"
	"github.com/devforge/be/internal/config"
	"github.com/devforge/be/internal/courses"
	"github.com/devforge/be/internal/labs"
)

func newRouter(cfg *config.Config, sqlDB *sql.DB, authMod *auth.Module, coursesMod *courses.Module, labsMod *labs.Module, chatMod *chat.Module, auditRec *audit.Recorder) (*gin.Engine, error) {
	r := gin.New()

	if err := setTrustedProxies(r, cfg.TrustedProxies); err != nil {
		return nil, err
	}
	r.Use(gin.Recovery(), requestLog(), cors(cfg.CORSOrigins))

	registerProbes(r, sqlDB)

	r.Static("/uploads", cfg.UploadDir)

	api := r.Group("/api")
	api.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "pong"}) })
	authMod.Routes(api)
	coursesMod.Routes(api, authMod.Required(), authMod.Optional(), authMod.AdminOnly())
	labsMod.Routes(r, api, authMod.Required(), authMod.AdminOnly())
	auditRec.Routes(api, authMod.Required(), authMod.AdminOnly())
	chatMod.Routes(r, api, authMod.Required())
	chatMod.AdminRoutes(api, authMod.Required(), authMod.AdminOnly())

	return r, nil
}

// setTrustedProxies decides whose X-Forwarded-For the server is willing to
// believe. Everything that identifies a caller reads c.ClientIP() — the
// per-address rate limit on the public share routes, audit_logs.ip, and the
// address shown on the logged-in-devices screen — so this is the one place that
// decides whether those three are about the caller or about the proxy.
//
// It comes from configuration rather than a constant because the answer changes
// with the deployment: nothing sits in front in development, nginx does in
// production. See TRUSTED_PROXIES in .env.example.
func setTrustedProxies(r *gin.Engine, proxies []string) error {
	if err := r.SetTrustedProxies(proxies); err != nil {
		return fmt.Errorf("trusted proxies: %w", err)
	}
	// Warn rather than refuse: a server that will not boot over a proxy setting
	// takes the site down in the wrong direction. The operator still gets told
	// what they gave away.
	if slices.Contains(proxies, "0.0.0.0/0") || slices.Contains(proxies, "::/0") {
		slog.Warn("TRUSTED_PROXIES trusts every peer",
			"consequence", "X-Forwarded-For is caller-controlled: the per-address rate limit and audit_logs.ip can be forged")
	}
	return nil
}

func registerProbes(r *gin.Engine, sqlDB *sql.DB) {
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db unreachable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
}
