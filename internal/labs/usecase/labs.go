package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/devforge/be/internal/labs/adapter/dockerx"
	"github.com/devforge/be/internal/labs/adapter/repo"
	"github.com/devforge/be/internal/labs/domain"
)

// checkTimeout bounds one run of a check script. The scripts are written by
// course authors, not students, but an author's `read` or a `cat` with no
// argument would otherwise hold a request open and leave an exec running inside
// the container until the session expired.
const checkTimeout = 10 * time.Second

type Labs struct {
	repo    *repo.SessionRepo
	grades  *repo.GradeRepo
	runtime *dockerx.Runtime
	ttl     time.Duration
}

func NewLabs(r *repo.SessionRepo, g *repo.GradeRepo, rt *dockerx.Runtime, ttl time.Duration) *Labs {
	return &Labs{repo: r, grades: g, runtime: rt, ttl: ttl}
}

type StartOutput struct {
	Session *domain.Session
	Spec    *domain.Spec
}

// Start claims the user's single session slot in the database before it asks
// docker for anything. Doing it the other way round would leave a container
// running with no row to reap it by if the insert then lost the race.
func (l *Labs) Start(ctx context.Context, userID int64, labSlug string) (StartOutput, error) {
	spec, err := l.repo.SpecBySlug(ctx, labSlug)
	if err != nil {
		return StartOutput{}, err
	}

	// Enrolment is checked here rather than in the handler, because this is the
	// one place every way of starting a lab passes through — and it is checked
	// before the container exists, so a refusal costs nothing to undo.
	enrolled, err := l.repo.IsEnrolled(ctx, userID, labSlug)
	if err != nil {
		return StartOutput{}, err
	}
	if !enrolled {
		return StartOutput{}, domain.ErrNotEnrolled
	}

	id, err := newSessionID()
	if err != nil {
		return StartOutput{}, err
	}
	s := &domain.Session{
		ID:         id,
		UserID:     userID,
		LabID:      spec.LabID,
		Status:     domain.StatusRunning,
		StartedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(l.ttl),
		LabSlug:    spec.LabSlug,
		CourseSlug: spec.CourseSlug,
	}
	if err := l.repo.Create(ctx, s); err != nil {
		return StartOutput{}, err
	}

	containerID, err := l.runtime.Create(ctx, id, spec.Image)
	if err != nil {
		// The row is holding the user's only slot for a container that does not
		// exist. Release it now or they cannot start anything until it expires.
		_, _ = l.repo.End(context.WithoutCancel(ctx), id, domain.StatusEnded)
		return StartOutput{}, err
	}
	if err := l.repo.SetContainer(ctx, id, containerID); err != nil {
		_ = l.runtime.Remove(context.WithoutCancel(ctx), containerID)
		_, _ = l.repo.End(context.WithoutCancel(ctx), id, domain.StatusEnded)
		return StartOutput{}, err
	}
	s.ContainerID = containerID
	return StartOutput{Session: s, Spec: spec}, nil
}

// Owned is the guard every session route goes through. Sessions are addressed by
// an unguessable id, but "hard to guess" is not the same as "checked", and the
// id travels in a URL where it ends up in logs and browser history.
func (l *Labs) Owned(ctx context.Context, sessionID string, userID int64) (*domain.Session, error) {
	s, err := l.repo.ByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if s.UserID != userID {
		// Same answer as a missing session: whether an id exists is not something
		// to confirm to somebody who does not own it.
		return nil, domain.ErrNotFound
	}
	return s, nil
}

// Live is Owned plus the checks that make a session usable right now, so the
// terminal and the check runner cannot attach to something already gone.
func (l *Labs) Live(ctx context.Context, sessionID string, userID int64) (*domain.Session, error) {
	s, err := l.Owned(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	if s.Status != domain.StatusRunning || time.Now().After(s.ExpiresAt) {
		return nil, domain.ErrNotRunning
	}
	if !l.runtime.Alive(ctx, s.ContainerID) {
		// The container went without the row being updated — a student who killed
		// their own PID 1, or a daemon restart. Settle the row so their slot frees.
		_, _ = l.repo.End(ctx, s.ID, domain.StatusEnded)
		return nil, domain.ErrNotRunning
	}
	return s, nil
}

func (l *Labs) Running(ctx context.Context, userID int64) (*domain.Session, error) {
	return l.repo.RunningByUser(ctx, userID)
}

// Stop is the student closing the lab themselves. The container goes first: if
// the row were settled first and the removal then failed, nothing would be
// looking for that container again.
func (l *Labs) Stop(ctx context.Context, sessionID string, userID int64) error {
	s, err := l.Owned(ctx, sessionID, userID)
	if err != nil {
		return err
	}
	return l.end(ctx, s)
}

// AdminStop is Stop without the ownership check. Same body underneath on
// purpose: an admin killing a container and a student ending their own session
// leave the system in one state, and two copies of this would be two chances for
// one of them to stop removing the container.
func (l *Labs) AdminStop(ctx context.Context, sessionID string) error {
	s, err := l.repo.ByID(ctx, sessionID)
	if err != nil {
		return err
	}
	return l.end(ctx, s)
}

// end removes the container and then settles the row. That order matters: a row
// marked ended while the container is still up leaves nothing looking for it
// again, and the reaper only sweeps rows that still say running.
func (l *Labs) end(ctx context.Context, s *domain.Session) error {
	if s.Status != domain.StatusRunning {
		return domain.ErrNotRunning
	}
	if s.ContainerID != "" {
		if err := l.runtime.Remove(ctx, s.ContainerID); err != nil {
			return err
		}
	}
	_, err := l.repo.End(ctx, s.ID, domain.StatusEnded)
	return err
}

// Submit is the student handing the lab in. Same order as Stop — the container
// goes first, because a row saying the attempt is over while the container is
// still up leaves nothing looking for it again. The answers are already on
// record, written as each check ran; this only closes the attempt over them.
func (l *Labs) Submit(
	ctx context.Context, sessionID string, userID int64,
) (*domain.Report, error) {
	s, err := l.Owned(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	if s.Status != domain.StatusRunning {
		return nil, domain.ErrNotRunning
	}
	// Every task has to be passed first. Checked here and not only in the
	// client: handing in an untouched lab is how you would read the answer key
	// out of the report and walk it into the next attempt.
	left, err := l.grades.RemainingTasks(ctx, s.ID, s.LabID)
	if err != nil {
		return nil, err
	}
	if left > 0 {
		return nil, domain.ErrIncomplete
	}
	if s.ContainerID != "" {
		if err := l.runtime.Remove(ctx, s.ContainerID); err != nil {
			return nil, err
		}
	}
	if err := l.grades.Submit(ctx, s.ID); err != nil {
		return nil, err
	}
	return l.grades.Report(ctx, s.ID)
}

// History is the student's own attempts. Scoped by user id rather than filtered
// afterwards: there is no request shape here that could ask for someone else's.
func (l *Labs) History(ctx context.Context, userID int64) ([]domain.HistoryRow, error) {
	return l.grades.History(ctx, userID)
}

// Report is one finished attempt, answer key included. Refused while the session
// is still running: the key is what the student is in there working out.
func (l *Labs) Report(
	ctx context.Context, sessionID string, userID int64,
) (*domain.Report, error) {
	s, err := l.Owned(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	if s.Status == domain.StatusRunning {
		return nil, domain.ErrStillRunning
	}
	return l.grades.Report(ctx, s.ID)
}

// Check grades one task against the student's own container and records the
// result. What decides is the state the script finds, not what was typed to get
// there: a task asking for a directory passes whether it was made with mkdir, a
// script, or an editor.
func (l *Labs) Check(
	ctx context.Context, sessionID string, userID, taskID int64, selected []int,
) (domain.Grade, error) {
	s, err := l.Live(ctx, sessionID, userID)
	if err != nil {
		return domain.Grade{}, err
	}
	task, err := l.grades.Task(ctx, taskID)
	if err != nil {
		return domain.Grade{}, err
	}
	// The session is what grants access to a container. A task from another lab
	// would be graded inside a container that was never meant to answer it, which
	// is a way to pass tasks by starting the one lab with the easiest setup.
	if task.LabID != s.LabID {
		return domain.Grade{}, domain.ErrTaskNotInLab
	}

	// A choice question is answered, not performed: there is nothing in the
	// container to look at, so no container is touched.
	if task.Kind == domain.KindChoice {
		return l.grades.Record(ctx, userID, s.ID, task, selected, sameAnswer(selected, task))
	}

	// A command task asks for something that leaves no trace — uname, which, cat
	// /proc/… — so the shell history is the only record that it was run.
	if task.Kind == domain.KindCommand {
		runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		defer cancel()
		history, _, err := l.runtime.Exec(runCtx, s.ContainerID,
			[]string{"/bin/sh", "-c", `cat "$HOME/.bash_history" 2>/dev/null`})
		if err != nil {
			if runCtx.Err() != nil && ctx.Err() == nil {
				return domain.Grade{}, domain.ErrCheckTimeout
			}
			return domain.Grade{}, err
		}
		return l.grades.Record(ctx, userID, s.ID, task, nil, ranCommand(history, task.ExpectedCommands))
	}

	passed := true
	if script := strings.TrimSpace(task.CheckScript); script != "" {
		runCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		defer cancel()
		// Through a shell, not exec'd directly: the scripts are written as shell
		// one-liners and lean on $HOME, globs and && the way an author expects.
		_, code, err := l.runtime.Exec(runCtx, s.ContainerID, []string{"/bin/sh", "-c", script})
		if err != nil {
			// The docker client wraps a cancelled context in its own error, so the
			// deadline is read off the context rather than unwrapped from it.
			if runCtx.Err() != nil && ctx.Err() == nil {
				return domain.Grade{}, domain.ErrCheckTimeout
			}
			return domain.Grade{}, err
		}
		passed = code == 0
	}
	return l.grades.Record(ctx, userID, s.ID, task, nil, passed)
}

// sameAnswer compares what was ticked against the key. Partial credit is not a
// thing here: a multi-answer question with one of three ticked is not half
// right, it is a different answer. Duplicates and out-of-range indexes are
// ignored rather than rejected — a client sending them is broken, and the worst
// it can do is fail its own submission.
func sameAnswer(selected []int, task *domain.Task) bool {
	picked := make(map[int]bool, len(selected))
	for _, i := range selected {
		if i >= 0 && i < task.OptionCount {
			picked[i] = true
		}
	}
	if len(picked) != len(task.CorrectOptions) {
		return false
	}
	for _, i := range task.CorrectOptions {
		if !picked[i] {
			return false
		}
	}
	return true
}

// ranCommand reports whether the history contains any of the accepted commands.
// Both sides go through the same normalisation, so spacing and the order the
// short flags were bundled in stop mattering: `ls -la /`, `ls -al /` and
// `ls -l -a /` are the one answer a student would call correct.
func ranCommand(history, expected string) bool {
	accepted := map[string]bool{}
	for _, line := range strings.Split(expected, "\n") {
		if line = normaliseCommand(line); line != "" {
			accepted[line] = true
		}
	}
	if len(accepted) == 0 {
		return false
	}
	for _, line := range strings.Split(history, "\n") {
		if accepted[normaliseCommand(line)] {
			return true
		}
	}
	return false
}

// normaliseCommand puts a command line in the one form grading compares. Beyond
// collapsing whitespace it splits bundled short flags and sorts each run of
// them, which is what makes `-la` and `-al` the same answer.
//
// Applying it to the author's line as well as the student's is what keeps this
// safe: the same input still normalises to the same output, so a command that
// matched before still matches. It can only widen what is accepted, never
// narrow it. What it widens to is `tar -cfz` counting as `tar -czf` — a line
// that would fail in the shell, so nobody types it on purpose.
//
// It cannot tell a bundle of short flags from a one-dash long option: `-name`
// is split into letters the same way `-la` is. That is noise rather than a bug,
// because both sides get the same treatment.
//
// ponytail: three variants are still the author's job, one per line —
// `--all` against `-a`, the `ll` alias in the lab bashrc against `ls -alF`, and
// `-n5` against `-n 5`. Each needs per-command knowledge this does not have; a
// flag alias table is the upgrade path if that ever becomes the common case.
func normaliseCommand(s string) string {
	var out []string
	// The start of the run of single-letter flags being collected, so a run is
	// sorted where it sits instead of flags migrating across their arguments:
	// sorting globally would reorder `find -type f -name x`.
	run := -1
	flush := func() {
		if run >= 0 {
			sort.Strings(out[run:])
			run = -1
		}
	}

	for _, tok := range strings.Fields(s) {
		for _, flag := range splitBundle(tok) {
			if len(flag) == 2 && flag[0] == '-' {
				if run < 0 {
					run = len(out)
				}
			} else {
				flush()
			}
			out = append(out, flag)
		}
	}
	flush()
	return strings.Join(out, " ")
}

// splitBundle explodes `-la` into `-l -a`, and leaves everything else alone.
// A digit anywhere in the token stops it: `head -n5` split into `-5 -n` would
// no longer look anything like `head -n 5`, which is the same command.
func splitBundle(tok string) []string {
	if len(tok) < 3 || tok[0] != '-' {
		return []string{tok}
	}
	for _, r := range tok[1:] {
		if !unicode.IsLetter(r) {
			return []string{tok}
		}
	}
	out := make([]string, 0, len(tok)-1)
	for _, r := range tok[1:] {
		out = append(out, "-"+string(r))
	}
	return out
}

// Stats is the admin overview. No user id: the role check is the whole of the
// authorisation here, and it happens on the route rather than in this call.
func (l *Labs) Stats(ctx context.Context) (*domain.Stats, error) {
	return l.grades.Stats(ctx)
}

// RunningSessions is the live-container list behind the kill button.
func (l *Labs) RunningSessions(ctx context.Context) ([]domain.RunningSession, error) {
	return l.grades.RunningSessions(ctx)
}

// PassedTaskIDs restores the ticks on the lab screen after a reload.
func (l *Labs) PassedTaskIDs(ctx context.Context, sessionID string) ([]int64, error) {
	return l.grades.PassedTaskIDs(ctx, sessionID)
}

// Attach hands back the shell stream for the websocket to pump. The exec id comes
// back with it because resizing the window needs it.
func (l *Labs) Attach(ctx context.Context, s *domain.Session) (io.ReadWriteCloser, string, error) {
	return l.runtime.Attach(ctx, s.ContainerID)
}

func (l *Labs) Resize(ctx context.Context, execID string, rows, cols uint) error {
	return l.runtime.Resize(ctx, execID, rows, cols)
}

// Remaining is what the client counts down from. Computed rather than stored so a
// clock that drifted between start and now cannot show a growing timer.
func Remaining(s *domain.Session) time.Duration {
	d := time.Until(s.ExpiresAt)
	if d < 0 {
		return 0
	}
	return d
}

func newSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
