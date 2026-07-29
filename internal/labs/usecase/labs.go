package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"github.com/devforge/be/internal/labs/adapter/dockerx"
	"github.com/devforge/be/internal/labs/adapter/repo"
	"github.com/devforge/be/internal/labs/domain"
)

type Labs struct {
	repo    *repo.SessionRepo
	runtime *dockerx.Runtime
	ttl     time.Duration
}

func NewLabs(r *repo.SessionRepo, rt *dockerx.Runtime, ttl time.Duration) *Labs {
	return &Labs{repo: r, runtime: rt, ttl: ttl}
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
		ID:        id,
		UserID:    userID,
		LabID:     spec.LabID,
		Status:    domain.StatusRunning,
		StartedAt: time.Now(),
		ExpiresAt: time.Now().Add(l.ttl),
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
