package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthcheck(t *testing.T) {
	t.Run("ready is a pass", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/readyz" {
				t.Errorf("probed %s, want /readyz", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		if err := healthcheck(srv.URL); err != nil {
			t.Fatalf("want pass, got %v", err)
		}
	})

	// The case the deploy actually rides on: the API is up and answering but
	// its database is gone. A 200-or-nothing check would call this healthy and
	// let CD keep a broken release.
	t.Run("db unreachable is a fail", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()
		if err := healthcheck(srv.URL); err == nil {
			t.Fatal("503 must fail the check")
		}
	})

	t.Run("nothing listening is a fail", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close()
		if err := healthcheck(url); err == nil {
			t.Fatal("refused connection must fail the check")
		}
	})
}
