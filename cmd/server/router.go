package main

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/auth"
	"github.com/devforge/be/internal/config"
	"github.com/devforge/be/internal/courses"
	"github.com/devforge/be/internal/labs"
)

func newRouter(cfg *config.Config, sqlDB *sql.DB, authMod *auth.Module, coursesMod *courses.Module, labsMod *labs.Module) (*gin.Engine, error) {
	r := gin.New()

	if err := r.SetTrustedProxies([]string{"127.0.0.1", "::1"}); err != nil {
		return nil, err
	}
	r.Use(gin.Recovery(), requestLog(), cors(cfg.CORSOrigins))

	registerProbes(r, sqlDB)

	r.Static("/uploads", cfg.UploadDir)

	api := r.Group("/api")
	api.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "pong"}) })
	authMod.Routes(api)
	coursesMod.Routes(api, authMod.Required(), authMod.Optional())
	labsMod.Routes(r, api, authMod.Required())

	return r, nil
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
