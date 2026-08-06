// Package quota counts what a user has spent today on something that costs
// money per call.
//
// It exists because the generator bills per request against a real account. A
// client with a retry loop and no cap is not a bug that shows up as a slow page:
// it shows up as an invoice. This is the only thing standing between the two.
package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/devforge/be/internal/labs/domain"
)

// Redis rather than a process counter: a counter in memory resets on deploy and
// is per-instance, so two replicas would quietly grant twice the cap.
type Redis struct {
	c     *redis.Client
	limit int
}

func New(c *redis.Client, limit int) *Redis { return &Redis{c: c, limit: limit} }

// Take counts one use and refuses past the daily limit.
//
// The key carries the date and expires after a day, so "daily" needs no reset
// job — tomorrow simply writes to a different key. The window is UTC calendar
// days rather than a rolling 24 hours: a person who understands "10 lần một
// ngày" can predict when it resets, which a rolling window never lets them do.
func (r *Redis) Take(ctx context.Context, userID int64) error {
	key := todayKey(userID)

	n, err := r.c.Incr(ctx, key).Result()
	if err != nil {
		// Refuse rather than allow. The failure mode of guessing wrong here is
		// asymmetric: a redis blip that opens the gate is billed to a card.
		return fmt.Errorf("đếm lượt dùng: %w", err)
	}
	if n == 1 {
		// Set only on the first increment of the day. Refreshing it on every
		// call would push the expiry forward forever on an active user.
		r.c.Expire(ctx, key, 48*time.Hour)
	}
	if n > int64(r.limit) {
		return domain.ErrAIQuota
	}
	return nil
}

// todayKey carries the date, so "daily" needs no reset job — tomorrow simply
// writes to a different key. UTC calendar days rather than a rolling window: a
// person told "10 lần một ngày" can predict when it resets, which a rolling
// 24 hours never lets them do.
func todayKey(userID int64) string {
	return fmt.Sprintf("aigen:%s:%d", time.Now().UTC().Format(time.DateOnly), userID)
}
