package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const allowedOrigin = "https://app.devforge.dev"

	tests := []struct {
		name       string
		method     string
		origin     string
		wantStatus int
		wantACAO   string
	}{
		{"allowlisted origin writes", http.MethodPost, allowedOrigin, http.StatusOK, allowedOrigin},
		{"same origin writes", http.MethodPost, "http://api.local", http.StatusOK, "http://api.local"},
		{"foreign origin blocked on write", http.MethodPost, "https://evil.example", http.StatusForbidden, ""},
		{"foreign origin allowed to read", http.MethodGet, "https://evil.example", http.StatusOK, ""},
		{"no origin passes", http.MethodPost, "", http.StatusOK, ""},
		{"preflight short-circuits", http.MethodOptions, allowedOrigin, http.StatusNoContent, allowedOrigin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(cors([]string{allowedOrigin}))
			r.Any("/", func(c *gin.Context) { c.Status(http.StatusOK) })

			req := httptest.NewRequest(tt.method, "/", nil)
			req.Host = "api.local"
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); got != tt.wantACAO {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tt.wantACAO)
			}
		})
	}
}
