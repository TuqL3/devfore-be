package quota

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/devforge/be/internal/labs/domain"
)

// Runs against a real redis because the whole value of this file is the two
// commands it sends. A fake would assert that Incr was called, which is exactly
// the thing that cannot be wrong; what can be wrong is the expiry landing on
// every call instead of the first, and only a real key shows that.
func client(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	c := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("REDIS_PASSWORD")})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		t.Skipf("cần redis chạy ở %s: %v", addr, err)
	}
	return c
}

// A user id no real account will collide with, so a leftover key from a failed
// run cannot make the next one pass or fail for the wrong reason.
const testUser = -424242

func TestTakeStopsAtTheLimit(t *testing.T) {
	c := client(t)
	ctx := context.Background()
	q := New(c, 3)

	key := todayKey(testUser)
	t.Cleanup(func() { c.Del(ctx, key) })
	c.Del(ctx, key)

	for i := 1; i <= 3; i++ {
		if err := q.Take(ctx, testUser); err != nil {
			t.Fatalf("lượt %d trong hạn mức bị chặn: %v", i, err)
		}
	}
	if err := q.Take(ctx, testUser); !errors.Is(err, domain.ErrAIQuota) {
		t.Fatalf("lượt thứ 4 phải bị chặn, got %v", err)
	}
	// Still refused after the limit — not a one-off that lets the next call in.
	if err := q.Take(ctx, testUser); !errors.Is(err, domain.ErrAIQuota) {
		t.Fatalf("lượt thứ 5 phải vẫn bị chặn, got %v", err)
	}
}

// The expiry is set only on the first increment. Refreshing it on every call
// would push the reset forward forever for an active user, so the cap would
// never actually reset for exactly the people it is meant to bound.
func TestExpiryIsNotPushedForwardByLaterCalls(t *testing.T) {
	c := client(t)
	ctx := context.Background()
	q := New(c, 10)

	key := todayKey(testUser)
	t.Cleanup(func() { c.Del(ctx, key) })
	c.Del(ctx, key)

	if err := q.Take(ctx, testUser); err != nil {
		t.Fatalf("Take: %v", err)
	}
	first, err := c.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if first <= 0 {
		t.Fatalf("lượt đầu không đặt hạn dùng cho key: ttl=%v", first)
	}

	// Shrink the window, then spend again. A second Expire would reset it to the
	// full 48h and this would catch it.
	if err := c.Expire(ctx, key, 30*time.Minute).Err(); err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if err := q.Take(ctx, testUser); err != nil {
		t.Fatalf("Take: %v", err)
	}
	after, err := c.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if after > 35*time.Minute {
		t.Fatalf("lượt sau đẩy hạn dùng lên lại: %v", after)
	}
}

// Two users must not share a budget.
func TestBudgetsAreScopedPerUser(t *testing.T) {
	c := client(t)
	ctx := context.Background()
	q := New(c, 1)

	other := int64(testUser - 1)
	t.Cleanup(func() { c.Del(ctx, todayKey(testUser), todayKey(other)) })
	c.Del(ctx, todayKey(testUser), todayKey(other))

	if err := q.Take(ctx, testUser); err != nil {
		t.Fatalf("Take: %v", err)
	}
	if err := q.Take(ctx, other); err != nil {
		t.Fatalf("người thứ hai bị chặn bởi hạn mức của người thứ nhất: %v", err)
	}
}
