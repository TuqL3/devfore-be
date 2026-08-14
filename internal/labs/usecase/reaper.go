package usecase

import (
	"context"
	"log/slog"
	"time"

	"github.com/devforge/be/internal/events"
	"github.com/devforge/be/internal/labs/domain"
)

const (
	reapEvery = 30 * time.Second
	// Enough to clear a burst without one sweep holding the database for long.
	// Anything left over is picked up thirty seconds later.
	reapBatch = 50
)

// Reap runs until ctx is cancelled. It is deliberately the only thing that
// removes an expired container: the deadline lives in the table, so a server
// that restarted, or a student who closed the tab an hour ago, are both handled
// by the same sweep with no in-memory state to lose.
func (l *Labs) Reap(ctx context.Context) {
	t := time.NewTicker(reapEvery)
	defer t.Stop()

	// One sweep before the first tick, so a restart cleans up whatever the
	// previous process left behind instead of waiting half a minute.
	l.reapOnce(ctx)
	// Housekeeping rides the same ticker rather than starting a second goroutine
	// for one DELETE a day. `sweepEvery` counts ticks, so the arithmetic stays in
	// one place if the reaper's own interval ever changes.
	const sweepEvery = int(24 * time.Hour / reapEvery)
	ticks := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.reapOnce(ctx)
			ticks++
			if ticks%sweepEvery == 0 && l.events != nil {
				l.events.Sweep(ctx)
			}
		}
	}
}

func (l *Labs) reapOnce(ctx context.Context) {
	due, err := l.repo.DueForReaping(ctx, reapBatch)
	if err != nil {
		slog.Error("reaper query", "err", err)
		return
	}
	for _, s := range due {
		if ctx.Err() != nil {
			return
		}
		l.reapOne(ctx, s)
	}
	if len(due) > 0 {
		slog.Info("reaper swept", "sessions", len(due))
	}
}

func (l *Labs) reapOne(ctx context.Context, s domain.Session) {
	if s.ContainerID != "" {
		// Running out of time is the most likely way a drill ends, so the
		// timeline has to survive this path as much as the other two.
		l.captureCommandLog(ctx, &s)
		if err := l.runtime.Remove(ctx, s.ContainerID); err != nil {
			// Leave the row running so the next sweep tries again. Marking it
			// expired here would lose the only pointer to a container that is
			// still holding memory.
			slog.Error("reaper remove", "session", s.ID, "container", s.ContainerID, "err", err)
			// The row stays running, so this repeats every sweep until somebody
			// looks. That repetition IS the signal: one line is a blip, the same
			// container every thirty seconds is a container nothing can remove.
			l.note(ctx, events.Event{
				Kind: events.KindOrphanContainer, ActorID: s.UserID, Subject: s.ContainerID,
				Detail: "không xoá được container hết hạn: " + err.Error(),
			})
			return
		}
	}
	ended, err := l.repo.End(ctx, s.ID, domain.StatusExpired)
	if err != nil {
		slog.Error("reaper end", "session", s.ID, "err", err)
		return
	}
	if ended {
		slog.Info("reaped lab session", "session", s.ID, "user", s.UserID, "lab", s.LabID)
	}
}
