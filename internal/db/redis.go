package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// OpenRedis dials and verifies the connection up front. Refresh sessions live
// in Redis, so a server that cannot reach it cannot authenticate anyone —
// better to fail at boot than to fail every login.
func OpenRedis(addr, password string, database int) (*redis.Client, error) {
	c := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       database,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("ping redis %s: %w", addr, err)
	}
	slog.Info("redis connected", "addr", addr)
	return c, nil
}
