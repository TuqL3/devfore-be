// Package events records the handful of things an admin would act on.
//
// It sits beside slog rather than replacing it. slog keeps everything, with the
// detail a developer needs and the volume that implies; this keeps the events
// somebody would open a screen to look at — a start that failed, a script that
// timed out, a host that ran out of room — in a shape a list can render.
//
// Every write is best-effort. These are records *about* failures, and a failure
// to record one must never turn a degraded request into a broken one: the
// caller is usually already in the middle of cleaning something up.
package events

import (
	"context"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// Kinds. Dotted, stable, and short enough to group by on a screen. Adding one
// needs no migration — the column is text and the screen counts what it finds.
const (
	KindStartFailed     = "lab.start_failed"
	KindCapacityRefused = "capacity.refused"
	KindCheckTimeout    = "check.timeout"
	KindIncidentFailed  = "incident.script_failed"
	KindHistoryLost     = "incident.history_lost"
	KindOrphanContainer = "reaper.orphan"
	KindRateLimited     = "public.rate_limited"
	KindAIRefused       = "ai.refused"
)

const (
	SeverityInfo  = "info"
	SeverityWarn  = "warn"
	SeverityError = "error"
)

// Retention is how long an event is kept.
//
// Thirty days answers "what happened last night" and "was this happening last
// week too", which is every question this table exists for. Longer would be a
// table that grows without anybody deciding to keep it.
const Retention = 30 * 24 * time.Hour

type Event struct {
	Kind     string
	Severity string
	// Zero when the event is about the host rather than a person.
	ActorID int64
	Subject string
	Detail  string
}

type Recorder struct{ db *gorm.DB }

func New(db *gorm.DB) *Recorder { return &Recorder{db: db} }

// Record writes one event and swallows its own failures.
//
// Deliberately takes a plain context rather than a gin one: half of these come
// from the reaper, which has no request behind it.
func (r *Recorder) Record(ctx context.Context, e Event) {
	if r == nil || r.db == nil {
		return
	}
	if e.Severity == "" {
		e.Severity = SeverityError
	}
	var actor any
	if e.ActorID > 0 {
		actor = e.ActorID
	}
	// context.WithoutCancel: the usual caller is unwinding a failed request, and
	// a cancelled context would drop exactly the events worth keeping.
	err := r.db.WithContext(context.WithoutCancel(ctx)).Exec(
		`INSERT INTO system_events (kind, severity, actor_id, subject, detail)
		 VALUES (?, ?, ?, ?, ?)`,
		e.Kind, e.Severity, actor, e.Subject, e.Detail,
	).Error
	if err != nil {
		slog.Error("record system event", "kind", e.Kind, "err", err)
	}
}

// Sweep deletes events past the retention window.
//
// Called from the reaper's own loop rather than from a scheduler of its own:
// there is already a goroutine ticking, and a second one for a daily DELETE is
// a lifetime to manage for no gain.
func (r *Recorder) Sweep(ctx context.Context) {
	if r == nil || r.db == nil {
		return
	}
	res := r.db.WithContext(ctx).Exec(
		`DELETE FROM system_events WHERE at < ?`, time.Now().Add(-Retention),
	)
	if res.Error != nil {
		slog.Error("sweep system events", "err", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		slog.Info("swept system events", "rows", res.RowsAffected)
	}
}

// Row is one event as the admin screen lists it, with the actor's name resolved
// — an id answers none of the questions somebody opens this screen with.
type Row struct {
	ID        int64
	At        time.Time
	Kind      string
	Severity  string
	ActorID   int64
	ActorName string
	Subject   string
	Detail    string
}

// List reads the feed, newest first, optionally narrowed to one kind or one
// severity. Both filters are exact matches: the screen offers what it found
// rather than a free-text box, so there is nothing to fuzzy-match.
func (r *Recorder) List(ctx context.Context, kind, severity string, limit int) ([]Row, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	out := []Row{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT e.id, e.at, e.kind, e.severity, e.subject, e.detail,
		       COALESCE(e.actor_id, 0)   AS actor_id,
		       COALESCE(u.username, '')  AS actor_name
		  FROM system_events e
		  LEFT JOIN users u ON u.id = e.actor_id
		 WHERE (? = '' OR e.kind = ?)
		   AND (? = '' OR e.severity = ?)
		 ORDER BY e.at DESC
		 LIMIT ?`, kind, kind, severity, severity, limit,
	).Scan(&out).Error
	return out, err
}

// KindCount is one row of the summary strip: how many of each kind in a window.
type KindCount struct {
	Kind     string
	Severity string
	N        int
}

// Summary counts the window by kind, so the screen can lead with "eleven failed
// starts today" instead of making somebody scroll a list to find that out.
func (r *Recorder) Summary(ctx context.Context, since time.Time) ([]KindCount, error) {
	out := []KindCount{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT kind, severity, count(*) AS n
		  FROM system_events
		 WHERE at >= ?
		 GROUP BY kind, severity
		 ORDER BY n DESC`, since,
	).Scan(&out).Error
	return out, err
}
