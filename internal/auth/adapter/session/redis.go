package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/devforge/be/internal/auth/domain"
	"github.com/devforge/be/internal/auth/usecase"
)

var _ usecase.SessionStore = (*Redis)(nil)

// Two keys per session:
//
//	sess:<id>    -> hash of user id + the device details the list screen shows
//	usess:<uid>  -> set of that user's session ids
//
// The second one exists so "log out everywhere" and "list my devices" are set
// reads instead of a keyspace scan, which would be O(all sessions on the
// server) for one person's request.
type Redis struct {
	c   *redis.Client
	ttl time.Duration
}

func New(c *redis.Client, ttl time.Duration) *Redis { return &Redis{c: c, ttl: ttl} }

func (r *Redis) TTL() time.Duration { return r.ttl }

func sessKey(id string) string { return "sess:" + id }

func userKey(uid int64) string { return "usess:" + strconv.FormatInt(uid, 10) }

// Hash fields. Short names because they are written on every refresh.
const (
	fUID     = "uid"
	fUA      = "ua"
	fIP      = "ip"
	fCreated = "created"
	fSeen    = "seen"
)

// maxUA caps what a client can make us store. The header is attacker-controlled
// and only ever gets displayed, so there is no reason to keep more than fits.
const maxUA = 256

func (r *Redis) Create(ctx context.Context, userID int64, meta usecase.SessionMeta) (domain.Session, error) {
	id, err := newID()
	if err != nil {
		return domain.Session{}, err
	}

	now := time.Now()
	created := meta.CreatedAt
	if created.IsZero() {
		created = now
	}
	s := domain.Session{
		ID:        id,
		UserID:    userID,
		UserAgent: truncate(meta.UserAgent, maxUA),
		IP:        meta.IP,
		CreatedAt: created,
		LastSeen:  now,
	}

	pipe := r.c.TxPipeline()
	pipe.HSet(ctx, sessKey(id), map[string]any{
		fUID:     userID,
		fUA:      s.UserAgent,
		fIP:      s.IP,
		fCreated: created.Unix(),
		fSeen:    now.Unix(),
	})
	pipe.Expire(ctx, sessKey(id), r.ttl)
	pipe.SAdd(ctx, userKey(userID), id)
	// The index has to outlive its newest member: without this a user who keeps
	// signing in would leave session keys behind that nothing can revoke.
	pipe.Expire(ctx, userKey(userID), r.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return domain.Session{}, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

func (r *Redis) Get(ctx context.Context, id string) (domain.Session, error) {
	if id == "" {
		return domain.Session{}, domain.ErrInvalidToken
	}
	m, err := r.c.HGetAll(ctx, sessKey(id)).Result()
	if err != nil {
		return domain.Session{}, fmt.Errorf("read session: %w", err)
	}
	// An expired or unknown key comes back as an empty map, not an error.
	if len(m) == 0 {
		return domain.Session{}, domain.ErrInvalidToken
	}
	return fromHash(id, m), nil
}

func (r *Redis) List(ctx context.Context, userID int64) ([]domain.Session, error) {
	ids, err := r.c.SMembers(ctx, userKey(userID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	pipe := r.c.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(ids))
	for i, id := range ids {
		cmds[i] = pipe.HGetAll(ctx, sessKey(id))
	}
	// Redis reports a missing key as an empty result rather than an error, so a
	// partial hit is normal here and Exec's error is not fatal on its own.
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("read sessions: %w", err)
	}

	out := make([]domain.Session, 0, len(ids))
	var stale []string
	for i, cmd := range cmds {
		m := cmd.Val()
		if len(m) == 0 {
			// The session expired but its id is still in the index. Redis has no
			// hook to remove it for us, so the read path cleans up after itself.
			stale = append(stale, ids[i])
			continue
		}
		out = append(out, fromHash(ids[i], m))
	}
	if len(stale) > 0 {
		r.c.SRem(ctx, userKey(userID), toAny(stale)...)
	}

	// Newest first: the device someone is looking for is the one they just used.
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	return out, nil
}

func (r *Redis) Revoke(ctx context.Context, id string) error {
	s, err := r.Get(ctx, id)
	// Already gone. Signing out twice, or with a stale cookie, is not an error.
	if errors.Is(err, domain.ErrInvalidToken) {
		return nil
	}
	if err != nil {
		return err
	}
	pipe := r.c.TxPipeline()
	pipe.Del(ctx, sessKey(id))
	pipe.SRem(ctx, userKey(s.UserID), id)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (r *Redis) RevokeAll(ctx context.Context, userID int64, keep string) error {
	ids, err := r.c.SMembers(ctx, userKey(userID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("list sessions: %w", err)
	}

	pipe := r.c.TxPipeline()
	queued := 0
	for _, id := range ids {
		if id == keep {
			continue
		}
		pipe.Del(ctx, sessKey(id))
		pipe.SRem(ctx, userKey(userID), id)
		queued++
	}
	// Nothing is left to point at, so drop the index itself rather than leaving
	// an empty set behind.
	if keep == "" {
		pipe.Del(ctx, userKey(userID))
		queued++
	}
	if queued == 0 {
		return nil
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("revoke sessions: %w", err)
	}
	return nil
}

func fromHash(id string, m map[string]string) domain.Session {
	uid, _ := strconv.ParseInt(m[fUID], 10, 64)
	return domain.Session{
		ID:        id,
		UserID:    uid,
		UserAgent: m[fUA],
		IP:        m[fIP],
		CreatedAt: unix(m[fCreated]),
		LastSeen:  unix(m[fSeen]),
	}
}

func unix(s string) time.Time {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(n, 0)
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// newID returns 256 bits of randomness. The id is the entire proof of a
// session, so it has to be unguessable in the same way a password is.
func newID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
