package verify

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/devforge/be/internal/auth/domain"
	"github.com/devforge/be/internal/auth/usecase"
)

var _ usecase.VerifyStore = (*Redis)(nil)

const maxAttempts = 5

type Redis struct{ c *redis.Client }

func New(c *redis.Client) *Redis { return &Redis{c: c} }

func codeKey(email string) string   { return "evc:" + email }
func resetKey(token string) string  { return "pwr:" + digest(token) }
func throttleKey(key string) string { return "thr:" + key }

const (
	fHash  = "hash"
	fTries = "tries"
)

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (r *Redis) PutCode(ctx context.Context, email, code string, ttl time.Duration) error {
	key := codeKey(email)
	pipe := r.c.TxPipeline()
	pipe.Del(ctx, key)
	pipe.HSet(ctx, key, map[string]any{fHash: digest(code), fTries: 0})
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("put code: %w", err)
	}
	return nil
}

func (r *Redis) CheckCode(ctx context.Context, email, code string) error {
	key := codeKey(email)
	m, err := r.c.HGetAll(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("read code: %w", err)
	}
	if len(m) == 0 {
		return domain.ErrInvalidCode
	}

	tries, _ := strconv.Atoi(m[fTries])
	if tries >= maxAttempts {
		r.c.Del(ctx, key)
		return domain.ErrTooManyAttempts
	}
	if subtle.ConstantTimeCompare([]byte(m[fHash]), []byte(digest(code))) != 1 {
		if n, err := r.c.HIncrBy(ctx, key, fTries, 1).Result(); err == nil && n >= maxAttempts {
			r.c.Del(ctx, key)
			return domain.ErrTooManyAttempts
		}
		return domain.ErrInvalidCode
	}

	r.c.Del(ctx, key)
	return nil
}

func (r *Redis) PutReset(ctx context.Context, token string, userID int64, ttl time.Duration) error {
	if err := r.c.Set(ctx, resetKey(token), userID, ttl).Err(); err != nil {
		return fmt.Errorf("put reset token: %w", err)
	}
	return nil
}

func (r *Redis) ConsumeReset(ctx context.Context, token string) (int64, error) {
	v, err := r.c.GetDel(ctx, resetKey(token)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, domain.ErrInvalidToken
	}
	if err != nil {
		return 0, fmt.Errorf("consume reset token: %w", err)
	}
	return v, nil
}

func (r *Redis) Attempt(ctx context.Context, key string, limit int, window time.Duration) error {
	k := "att:" + key
	n, err := r.c.Incr(ctx, k).Result()
	if err != nil {
		return fmt.Errorf("attempt: %w", err)
	}
	if n == 1 {
		r.c.Expire(ctx, k, window)
	}
	if n > int64(limit) {
		return domain.ErrRateLimited
	}
	return nil
}

func (r *Redis) ClearAttempts(ctx context.Context, key string) error {
	if err := r.c.Del(ctx, "att:"+key).Err(); err != nil {
		return fmt.Errorf("clear attempts: %w", err)
	}
	return nil
}

func (r *Redis) Throttle(ctx context.Context, key string, window time.Duration) error {
	ok, err := r.c.SetNX(ctx, throttleKey(key), 1, window).Result()
	if err != nil {
		return fmt.Errorf("throttle: %w", err)
	}
	if !ok {
		return domain.ErrRateLimited
	}
	return nil
}
