package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/devforge/be/internal/labs/domain"
)

// Stop and AdminStop share end(), so the guard that a finished session cannot be
// finished again is written once. If it stopped firing, an admin pressing kill
// on a row that just expired would call docker with a container id that is gone
// and report a server error for something that already happened.
//
// The check runs before the repo or the runtime is touched, so a zero-value Labs
// proves it — reaching either would panic, which is the failure this reports.
func TestEndRefusesASessionThatIsOver(t *testing.T) {
	l := &Labs{}
	ctx := context.Background()

	for _, status := range []domain.Status{
		domain.StatusEnded, domain.StatusExpired, domain.StatusSubmitted,
	} {
		s := &domain.Session{ID: "x", Status: status}
		if err := l.end(ctx, s); !errors.Is(err, domain.ErrNotRunning) {
			t.Fatalf("end on %s = %v, want ErrNotRunning", status, err)
		}
	}
}
