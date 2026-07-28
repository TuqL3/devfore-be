package usecase

import (
	"context"
	"testing"

	"github.com/devforge/be/internal/auth/domain"
)

type revokedSessions struct {
	SessionStore
	live map[string]bool
}

func (r revokedSessions) Get(_ context.Context, id string) (domain.Session, error) {
	if !r.live[id] {
		return domain.Session{}, domain.ErrInvalidToken
	}
	return domain.Session{ID: id, UserID: 7}, nil
}

type parsingTokens struct {
	TokenIssuer
	sid string
}

func (p parsingTokens) ParseAccess(string) (int64, []string, string, error) {
	return 7, []string{"user"}, p.sid, nil
}

// A signed access token whose session was revoked elsewhere must stop working
// immediately, not at token expiry.
func TestAuthorizeRejectsRevokedSession(t *testing.T) {
	store := revokedSessions{live: map[string]bool{"live-sid": true}}

	a := NewAuth(nil, parsingTokens{sid: "live-sid"}, nil, nil, store, nil, nil, Verification{})
	if _, _, sid, err := a.Authorize(context.Background(), "tok"); err != nil || sid != "live-sid" {
		t.Fatalf("live session: got sid=%q err=%v, want sid=live-sid err=nil", sid, err)
	}

	a = NewAuth(nil, parsingTokens{sid: "revoked-sid"}, nil, nil, store, nil, nil, Verification{})
	if _, _, _, err := a.Authorize(context.Background(), "tok"); err == nil {
		t.Fatal("revoked session: got nil error, want rejection")
	}
}
