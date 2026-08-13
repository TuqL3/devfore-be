package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/audit"
	"github.com/devforge/be/internal/auth"
	"github.com/devforge/be/internal/chat"
	"github.com/devforge/be/internal/config"
	"github.com/devforge/be/internal/courses"
	"github.com/devforge/be/internal/db"
	"github.com/devforge/be/internal/events"
	"github.com/devforge/be/internal/labs"
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
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel(cfg.LogLevel),
	})))

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
		CookieSecure:       cfg.IsProd(),
		SMTPHost:           cfg.SMTPHost,
		SMTPPort:           cfg.SMTPPort,
		SMTPUser:           cfg.SMTPUser,
		SMTPPass:           cfg.SMTPPass,
		MailFrom:           cfg.MailFrom,
		VerifyCodeTTL:      cfg.VerifyCodeTTL,
		ResetTokenTTL:      cfg.ResetTokenTTL,
		ResendCooldown:     cfg.ResendCooldown,
	})
	// The room's connections are capped at one access-token lifetime, so a
	// banned account cannot keep talking on a socket opened before the ban.
	chatMod := chat.New(gdb, chat.Config{
		AllowedOrigins: cfg.CORSOrigins,
		SessionWindow:  cfg.AccessTTL,
	})
	coursesMod := courses.New(gdb, courses.Config{
		UploadDir: cfg.UploadDir,
		PublicURL: cfg.PublicURL,
	})

	// The reaper has to outlive every request but die with the process, so it
	// hangs off the same signal context the http server shuts down on.
	reaperCtx, stopReaper := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopReaper()

	labsMod, err := labs.New(reaperCtx, gdb, rdb, labs.Config{
		DockerHost:      cfg.DockerHost,
		SessionTTL:      cfg.LabSessionTTL,
		AllowedOrigins:  cfg.CORSOrigins,
		OpenRouterKey:   cfg.OpenRouterKey,
		OpenRouterModel: cfg.OpenRouterModel,
		PublicURL:       cfg.PublicURL,
		FrontendURL:     cfg.FrontendURL,
		AIDailyLimit:    cfg.AIDailyLimit,
		PublicRateLimit: cfg.PublicRateLimit,
		MaxContainers:   cfg.MaxContainers,
	})
	if err != nil {
		return err
	}
	defer labsMod.Close()
	labsMod.StartReaper(reaperCtx)

	// Wired after both modules exist rather than into either constructor: audit
	// is written to by both and owned by neither, and passing it in would force
	// one of them to be built first for no reason other than this.
	auditRec := audit.New(gdb)
	authMod.SetAudit(auditRec)
	labsMod.SetAudit(auditRec)
	chatMod.SetAudit(auditRec)

	// Two recorders, two different questions. Audit answers "who did this to
	// whom" and is written only by deliberate admin actions; events answer "what
	// went wrong" and are written by the code paths that fail.
	eventRec := events.New(gdb)
	labsMod.SetEvents(eventRec)

	if cfg.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}
	r, err := newRouter(cfg, sqlDB, authMod, coursesMod, labsMod, chatMod, auditRec)
	if err != nil {
		return err
	}

	return serve(&http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}, cfg.Env)
}

func serve(srv *http.Server, env string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http listening", "addr", srv.Addr, "env", env)
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
