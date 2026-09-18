package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The address the server settles on is what the public rate limit counts and
// what audit_logs.ip records, so the two halves both matter: a proxy we put
// there must be believed, and nobody else may claim to be somebody else.
func TestTrustedProxiesDecideTheCallerAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// What production sets once the edge is in front — see TRUSTED_PROXIES.
	behindEdge := []string{"127.0.0.1", "::1", "172.16.0.0/12"}
	// The default: nothing in front, so no header is believed.
	loopbackOnly := []string{"127.0.0.1", "::1"}

	tests := []struct {
		name    string
		trusted []string
		remote  string // peer the request actually arrived from
		fwd     string // X-Forwarded-For it carried
		want    string
	}{
		{
			name:    "default ignores a header nobody was asked for",
			trusted: loopbackOnly,
			remote:  "203.0.113.9:5555",
			fwd:     "1.2.3.4",
			want:    "203.0.113.9",
		},
		{
			name:    "the edge on the compose bridge is believed",
			trusted: behindEdge,
			remote:  "172.18.0.4:40000",
			fwd:     "203.0.113.9",
			want:    "203.0.113.9",
		},
		{
			// The regression this whole knob exists for: with only loopback
			// trusted, every request through the edge answers with the edge's
			// own address and the per-address rate limit becomes one global
			// bucket.
			name:    "the edge unlisted swallows every caller into one address",
			trusted: loopbackOnly,
			remote:  "172.18.0.4:40000",
			fwd:     "203.0.113.9",
			want:    "172.18.0.4",
		},
		{
			name:    "a stranger cannot pick an address by sending the header",
			trusted: behindEdge,
			remote:  "203.0.113.9:5555",
			fwd:     "10.0.0.1",
			want:    "203.0.113.9",
		},
		{
			name:    "no header, no proxy, nothing to decide",
			trusted: loopbackOnly,
			remote:  "203.0.113.9:5555",
			want:    "203.0.113.9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			if err := setTrustedProxies(r, tt.trusted); err != nil {
				t.Fatalf("setTrustedProxies: %v", err)
			}
			r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remote
			if tt.fwd != "" {
				req.Header.Set("X-Forwarded-For", tt.fwd)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if got := w.Body.String(); got != tt.want {
				t.Errorf("ClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetTrustedProxiesRejectsGarbage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if err := setTrustedProxies(gin.New(), []string{"not-an-address"}); err == nil {
		t.Fatal("want an error for an unparseable proxy address, got nil")
	}
}
