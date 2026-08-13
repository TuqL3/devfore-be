// Package ratelimit caps how often one address may hit a route that answers
// without a caller identity.
//
// It exists because two routes on this platform are reachable with no account:
// the shared drill page and the day's challenge. Both are meant to be pasted
// into places where a lot of people click at once, and both run a query per
// request. The cap is what stands between a good day and a database with
// nothing left to give the students who are signed in.
package ratelimit

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/devforge/be/internal/events"
)

type Redis struct {
	c      *redis.Client
	limit  int
	window time.Duration
	prefix string
	rec    *events.Recorder
}

func New(c *redis.Client, limit int, window time.Duration, prefix string) *Redis {
	return &Redis{c: c, limit: limit, window: window, prefix: prefix}
}

// SetEvents makes the cap visible to an admin. Optional, like everywhere else
// this recorder is wired: without it the cap still works, quietly.
func (r *Redis) SetEvents(rec *events.Recorder) {
	if r != nil {
		r.rec = rec
	}
}

// Middleware counts one request per caller address and refuses past the limit.
//
// Fixed window rather than a sliding one: a sliding window needs a sorted set
// per address and a trim on every call, to buy an edge case that matters when
// the limit is 5 and not when it is 60. The seam it leaves — a burst either side
// of a boundary can reach twice the limit — is a burst this is happy to serve.
//
// **Opens the gate when redis is unreachable**, which is the opposite of what
// the AI quota does with the same client, and deliberately so. There the failure
// costs money, so guessing wrong has to be refused; here the failure costs a
// public page that somebody just clicked a link to reach, and a stranger's first
// impression is worth more than a few extra queries during an outage.
func (r *Redis) Middleware() gin.HandlerFunc {
	if r == nil || r.c == nil || r.limit <= 0 {
		// No redis configured: no cap. Said explicitly rather than left to a nil
		// dereference three months from now.
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		key := r.prefix + ":" + c.ClientIP() + ":" +
			strconv.FormatInt(time.Now().Unix()/int64(r.window.Seconds()), 10)

		n, err := r.c.Incr(c.Request.Context(), key).Result()
		if err != nil {
			slog.Warn("rate limit unavailable", "prefix", r.prefix, "err", err)
			c.Next()
			return
		}
		if n == 1 {
			// Only on the first hit of a window. Refreshing it every call would
			// push the expiry forward for as long as somebody keeps knocking, and
			// the key would outlive the window it describes.
			r.c.Expire(c.Request.Context(), key, r.window+time.Second)
		}
		if n > int64(r.limit) {
			// Recorded on the FIRST refusal of a window only. Writing a row per
			// refused request would mean a flood arrives as a flood of rows, and
			// the table meant to describe the incident becomes part of it. One row
			// per address per window is the same signal at a thousandth the cost.
			if n == int64(r.limit)+1 && r.rec != nil {
				r.rec.Record(c.Request.Context(), events.Event{
					Kind: events.KindRateLimited, Severity: events.SeverityWarn,
					Subject: c.ClientIP(),
					Detail: "vượt " + strconv.Itoa(r.limit) + " lượt/" +
						r.window.String() + " trên " + c.Request.URL.Path,
				})
			}
			c.Header("Retry-After", strconv.Itoa(int(r.window.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests,
				gin.H{"error": "quá nhiều yêu cầu, thử lại sau ít giây"})
			return
		}
		c.Next()
	}
}
