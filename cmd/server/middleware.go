package main

import (
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
)

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

func cors(allowed []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		trusted := origin != "" && (slices.Contains(allowed, origin) || sameOrigin(origin, c.Request.Host))

		if trusted {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Accept-Language")
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Max-Age", "600")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		if origin != "" && !trusted && !safeMethod(c.Request.Method) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "cross-origin request rejected"})
			return
		}
		c.Next()
	}
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Host == host
}
