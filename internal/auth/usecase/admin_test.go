package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/devforge/be/internal/auth/domain"
)

// The self-target refusal is the whole of the lockout protection: because the
// caller is an active admin by the time these run, a rule that stops them
// removing themselves is what guarantees an active admin is always left. If it
// ever stops firing, an admin can revoke their own role or ban their own account
// and nobody can reach the screen that would undo it.
//
// Both checks run before any dependency is touched, so a zero-value Auth is
// enough to prove it — reaching a nil repo would panic, which is the failure
// this test would report.
func TestAdminCannotModerateSelf(t *testing.T) {
	a := &Auth{}
	ctx := context.Background()

	if err := a.SetAdmin(ctx, 7, 7, false); !errors.Is(err, domain.ErrSelfTarget) {
		t.Fatalf("SetAdmin on self = %v, want ErrSelfTarget", err)
	}
	if err := a.SetBanned(ctx, 7, 7, true, "vì thế"); !errors.Is(err, domain.ErrSelfTarget) {
		t.Fatalf("SetBanned on self = %v, want ErrSelfTarget", err)
	}
	// Unbanning yourself is refused on the same rule rather than waved through
	// as harmless: an admin who can reach this endpoint is not banned, so the
	// only thing the call could be is a mistake aimed at the wrong row.
	if err := a.SetBanned(ctx, 7, 7, false, ""); !errors.Is(err, domain.ErrSelfTarget) {
		t.Fatalf("SetBanned unban on self = %v, want ErrSelfTarget", err)
	}
}
