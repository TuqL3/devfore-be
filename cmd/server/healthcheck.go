package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

// The production image is distroless: no shell, no wget, nothing compose can
// exec to probe the port. So the binary probes itself, and CD's rollback
// condition hangs off this exit code.
//
// /readyz rather than /healthz: a process that answers but cannot reach
// Postgres is not a deploy worth keeping, and telling those two apart is the
// whole point of running the check.
func healthcheck(base string) error {
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(base + "/readyz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("readyz answered %s", resp.Status)
	}
	return nil
}

func localBase() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return "http://127.0.0.1:" + port
}
