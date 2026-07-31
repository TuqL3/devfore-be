package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

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
	if s.Status != domain.StatusRunning {
		return domain.ErrNotRunning
	}
	if s.ContainerID != "" {
		if err := l.runtime.Remove(ctx, s.ContainerID); err != nil {
			return err
		}
	}
	_, err = l.repo.End(ctx, s.ID, domain.StatusEnded)
	return err
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
		return l.grades.Record(ctx, userID, s.ID, task, sameAnswer(selected, task))
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
		return l.grades.Record(ctx, userID, s.ID, task, ranCommand(history, task.ExpectedCommands))
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
	return l.grades.Record(ctx, userID, s.ID, task, passed)
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
// Both sides are whitespace-normalised, so `ls   -la  /` matches `ls -la /` —
// the shell does not care about the spacing and neither should the grading.
// Everything else is compared literally: `ls -la /` and `ls -al /` are different
// answers, and an author who accepts both writes both.
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

func normaliseCommand(s string) string { return strings.Join(strings.Fields(s), " ") }

// PassedTaskIDs restores the ticks on the lab screen after a reload.
func (l *Labs) PassedTaskIDs(ctx context.Context, userID, labID int64) ([]int64, error) {
	return l.grades.PassedTaskIDs(ctx, userID, labID)
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
