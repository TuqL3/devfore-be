package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/devforge/be/internal/labs/domain"
)

// incidentTimeout bounds building and then breaking the service. Longer than a
// check script because this one writes files and starts a daemon, still short
// enough that a scenario with a `read` in it fails the start rather than holding
// the request open until the browser gives up.
const incidentTimeout = 20 * time.Second

// drillDeadline is how long the session gets before the reaper takes it.
//
// For an ordinary lab that is the platform's container TTL: the number is about
// how long a container may sit on the host, and the lab's own duration is only a
// hint to the student about how long the material takes.
//
// For a drill the two swap places. The time limit *is* the challenge — an outage
// you have all hour to fix teaches nothing about working under one — so the
// lab's own minutes decide, and running out of them is a real outcome with its
// own report. Still capped by the TTL: a drill must never outlive what the host
// is willing to give a container.
func drillDeadline(spec *domain.Spec, ttl time.Duration) time.Duration {
	if !spec.IsIncident || spec.DurationMinutes <= 0 {
		return ttl
	}
	if d := time.Duration(spec.DurationMinutes) * time.Minute; d < ttl {
		return d
	}
	return ttl
}

// applyIncident draws a fault for the session and puts the container into it.
//
// A lab with no scenarios is an ordinary lab, so ErrNoIncident is the answer for
// every container lab on the platform and is not an error here. Anything else is:
// a student must never be dropped into a drill that half-broke, because the
// evidence they are about to reason from would be a mix of the author's fault and
// a failure nobody intended.
//
// `want` names a scenario instead of drawing one, which is what a shared link and
// the daily drill both arrive with. Zero means draw. A named scenario that is not
// a live one of this lab is refused rather than replaced by a draw: the promise a
// shared link makes is that it hands you the same fault, and quietly handing over
// a different one would break that promise without ever saying so.
func (l *Labs) applyIncident(
	ctx context.Context, s *domain.Session, spec *domain.Spec, want int64,
) error {
	var inc *domain.Incident
	var err error
	if want > 0 {
		inc, err = l.repo.IncidentForLab(ctx, spec.LabID, want)
	} else {
		inc, err = l.repo.PickIncident(ctx, spec.LabID)
	}
	if errors.Is(err, domain.ErrNoIncident) {
		return nil
	}
	if err != nil {
		return err
	}

	// Setup and break run as one shell under `set -e`, in that order. Separately
	// would let a failed setup be followed by a break that "succeeds" against a
	// service that was never there — a container broken in a way the author never
	// wrote and the student cannot fix.
	script := "set -e\n" + spec.IncidentSetup + "\n" + inc.BreakScript
	runCtx, cancel := context.WithTimeout(ctx, incidentTimeout)
	defer cancel()
	out, code, err := l.runtime.Exec(runCtx, s.ContainerID, []string{"/bin/sh", "-c", script})
	if err != nil {
		if runCtx.Err() != nil && ctx.Err() == nil {
			return fmt.Errorf("kịch bản sự cố %d chạy quá %s", inc.ID, incidentTimeout)
		}
		return fmt.Errorf("chạy kịch bản sự cố %d: %w", inc.ID, err)
	}
	if code != 0 {
		// The output goes to the log rather than to the student: it is the answer
		// key talking, and the line it failed on names the fault outright.
		slog.Error("incident script failed",
			"incident", inc.ID, "lab", spec.LabID, "code", code, "out", out)
		return fmt.Errorf("kịch bản sự cố %d thoát với mã %d", inc.ID, code)
	}

	if err := l.repo.SetIncident(ctx, s.ID, inc.ID); err != nil {
		return err
	}
	// Both fields, not just the id. This session was built in memory rather than
	// read back through sessionSelect, so nothing else is going to fill the rate
	// in — and the response to Start is the one the drill screen opens with.
	s.IncidentID = &inc.ID
	s.IncidentRPS = inc.RPS
	return nil
}

// captureCommandLog stores what the student typed, for a drill that is ending.
//
// Called before the container is removed, by every path that ends a session —
// handing in, stopping, and the reaper — because a timeline that only survives
// one of the three ways out is a feature that works until the clock runs out.
//
// A failure here is logged and swallowed on purpose. The caller is in the middle
// of removing a container, and losing the record of a drill is a smaller harm
// than leaving that container running because reading a file went wrong.
func (l *Labs) captureCommandLog(ctx context.Context, s *domain.Session) {
	if s.IncidentID == nil || s.ContainerID == "" {
		return
	}
	runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	history, _, err := l.runtime.Exec(runCtx, s.ContainerID,
		[]string{"/bin/sh", "-c", `cat "$HOME/.bash_history" 2>/dev/null`})
	if err != nil {
		slog.Error("incident history read", "session", s.ID, "err", err)
		return
	}
	if strings.TrimSpace(history) == "" {
		return
	}
	if err := l.repo.SaveCommandLog(ctx, s.ID, history); err != nil {
		slog.Error("incident history save", "session", s.ID, "err", err)
	}
}

// attachIncident adds the drill half of a report: the fault, what it cost, and
// the attempts made against it. A session that drew nothing is left exactly as it
// was, which is every session of every lab that is not an incident lab.
//
// Failures are logged rather than returned. The report is already assembled and
// correct at this point — refusing to show a student their answers because the
// scenario row could not be read would trade the whole screen for a detail.
func (l *Labs) attachIncident(ctx context.Context, s *domain.Session, rep *domain.Report) {
	if s.IncidentID == nil {
		return
	}
	inc, err := l.repo.IncidentByID(ctx, *s.IncidentID)
	if err != nil {
		slog.Error("incident report read", "session", s.ID, "incident", *s.IncidentID, "err", err)
		return
	}
	log, err := l.repo.CommandLog(ctx, s.ID)
	if err != nil {
		slog.Error("incident log read", "session", s.ID, "err", err)
	}

	out := &domain.IncidentReport{
		Title:       inc.Title,
		RevealMD:    inc.RevealMD,
		RPS:         inc.RPS,
		RecoveredAt: recoveredAt(rep),
		Timeline:    ParseTimeline(log),
	}
	if out.RecoveredAt != nil {
		down := out.RecoveredAt.Sub(rep.StartedAt)
		out.DowntimeSeconds = int(down.Seconds())
		out.RequestsFailed = RequestsFailed(down, inc.RPS)
	}
	rep.Incident = out
}

// recoveredAt is when the student declared the service healthy: the moment the
// last of the lab's tasks went green.
//
// The last rather than the first, because a lab may ask for more than the service
// being up, and an outage is not over while any part of the ask is still failing.
// Nil unless every task passed — a drill that ran out of time has no recovery, and
// dating one from a partial pass would be the report inventing a moment.
func recoveredAt(rep *domain.Report) *time.Time {
	var last *time.Time
	for _, a := range rep.Answers {
		if a.Passed == nil || !*a.Passed || a.AnsweredAt == nil {
			return nil
		}
		if last == nil || a.AnsweredAt.After(*last) {
			last = a.AnsweredAt
		}
	}
	if len(rep.Answers) == 0 {
		return nil
	}
	return last
}

// ParseTimeline turns a bash history file into the ordered list of attempts the
// report shows.
//
// The format is bash's own: with HISTTIMEFORMAT set, every command is preceded by
// a `#<epoch>` line, and without it the commands stand alone. Both shapes are
// read, because a session that started before the rc file carried the setting
// still has a history worth showing — just without times.
//
// A `#` line with anything but digits after it is a comment the student typed,
// not a stamp, so it stays in the list as a command. Getting that backwards would
// silently delete lines from the record of what somebody did.
func ParseTimeline(history string) []domain.TimelineEntry {
	entries := []domain.TimelineEntry{}
	var pending time.Time
	for _, line := range strings.Split(history, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if sec, ok := historyStamp(line); ok {
			pending = time.Unix(sec, 0).UTC()
			continue
		}
		entries = append(entries, domain.TimelineEntry{At: pending, Command: line})
		// One stamp belongs to one command. Continuation lines of a multi-line
		// command carry no stamp of their own, and dating them from the line above
		// would claim a precision the file does not have.
		pending = time.Time{}
	}
	return entries
}

// historyStamp reads `#1786400000` and nothing else. Leading `#` plus digits, no
// spaces, no sign: exactly what bash writes and narrow enough that a student's
// own `# note to self` is never mistaken for one.
func historyStamp(line string) (int64, bool) {
	if len(line) < 2 || line[0] != '#' {
		return 0, false
	}
	sec, err := strconv.ParseInt(line[1:], 10, 64)
	if err != nil || sec <= 0 {
		return 0, false
	}
	return sec, true
}

// RequestsFailed is the cost of an outage that lasted d, at the scenario's
// assumed rate. Simulated end to end — the rate is a number the author typed —
// so whatever displays it has to say so, the same way the pipeline simulator
// labels its seconds.
//
// Negative durations answer zero rather than a negative count: a recovery
// recorded before the start is a clock problem, and a report claiming minus four
// thousand requests would be read as a bug in the drill rather than in the clock.
func RequestsFailed(d time.Duration, rps int) int {
	if d <= 0 || rps <= 0 {
		return 0
	}
	return int(d.Seconds()) * rps
}
