package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devforge/be/internal/auth/domain"
)

type memUsers struct {
	UserRepository
	byEmail map[string]*domain.User
	pwSet   map[int64]string
}

func (m memUsers) ByEmail(_ context.Context, email string) (*domain.User, error) {
	u, ok := m.byEmail[email]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

func (m memUsers) ByID(_ context.Context, id int64) (*domain.User, error) {
	for _, u := range m.byEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m memUsers) SetStatus(_ context.Context, id int64, s domain.Status) error {
	for _, u := range m.byEmail {
		if u.ID == id {
			u.Status = s
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m memUsers) UpdatePassword(_ context.Context, id int64, hash string) error {
	m.pwSet[id] = hash
	return nil
}

// memCodes is the honest half of the store: it holds one code and one token and
// refuses anything else, which is all these flows read back.
type memCodes struct {
	code      string
	resetFor  int64
	hasReset  bool
	throttle  bool
	attempts  int
	attemptTo int // 0 means unlimited
	cleared   int
}

func (c *memCodes) Attempt(_ context.Context, _ string, limit int, _ time.Duration) error {
	c.attempts++
	if c.attemptTo > 0 && c.attempts > c.attemptTo {
		return domain.ErrRateLimited
	}
	if c.attempts > limit {
		return domain.ErrRateLimited
	}
	return nil
}

func (c *memCodes) ClearAttempts(context.Context, string) error {
	c.attempts = 0
	c.cleared++
	return nil
}

func (c *memCodes) PutCode(context.Context, string, string, time.Duration) error { return nil }

func (c *memCodes) CheckCode(_ context.Context, _, code string) error {
	if code != c.code {
		return domain.ErrInvalidCode
	}
	c.code = ""
	return nil
}

func (c *memCodes) PutReset(context.Context, string, int64, time.Duration) error { return nil }

func (c *memCodes) ConsumeReset(context.Context, string) (int64, error) {
	if !c.hasReset {
		return 0, domain.ErrInvalidToken
	}
	c.hasReset = false
	return c.resetFor, nil
}

func (c *memCodes) Throttle(context.Context, string, time.Duration) error {
	if c.throttle {
		return domain.ErrRateLimited
	}
	return nil
}

type countingSessions struct {
	SessionStore
	revokedAll int
}

func (countingSessions) Create(_ context.Context, uid int64, _ SessionMeta) (domain.Session, error) {
	return domain.Session{ID: "sid", UserID: uid}, nil
}
func (countingSessions) TTL() time.Duration { return time.Hour }
func (s *countingSessions) RevokeAll(context.Context, int64, string) error {
	s.revokedAll++
	return nil
}

type staticHasher struct {
	PasswordHasher
	out string
}

func (h staticHasher) Hash(string) (string, error) { return h.out, nil }

func TestVerifyEmailActivatesOnlyWithTheRightCode(t *testing.T) {
	pending := &domain.User{ID: 1, Email: "a@b.io", Status: domain.StatusPending}
	users := memUsers{byEmail: map[string]*domain.User{"a@b.io": pending}}
	codes := &memCodes{code: "123456"}
	a := NewAuth(users, fakeTokens{}, nil, nil, &countingSessions{}, codes, nil, Verification{})

	if _, err := a.VerifyEmail(context.Background(), "a@b.io", "000000", SessionMeta{}); !errors.Is(err, domain.ErrInvalidCode) {
		t.Fatalf("wrong code: got %v, want ErrInvalidCode", err)
	}
	if pending.Status != domain.StatusPending {
		t.Fatalf("wrong code changed status to %q", pending.Status)
	}

	// Mixed case in, lowercased lookup out — the address is stored lowercased.
	out, err := a.VerifyEmail(context.Background(), "A@B.io", "123456", SessionMeta{})
	if err != nil {
		t.Fatalf("right code: %v", err)
	}
	if pending.Status != domain.StatusActive {
		t.Fatalf("status after verify = %q, want active", pending.Status)
	}
	if out.Credentials.AccessToken == "" {
		t.Fatal("verify did not sign the user in")
	}

	// The code is spent, so replaying it must not work a second time.
	if _, err := a.VerifyEmail(context.Background(), "a@b.io", "123456", SessionMeta{}); err == nil {
		t.Fatal("replayed code succeeded")
	}
}

// Burning a code costs five guesses; requesting a fresh one resets that. The
// per-source cap is what stops that loop, so it must fire even when every
// address tried is different.
func TestVerifyEmailCapsAttemptsPerSource(t *testing.T) {
	users := memUsers{byEmail: map[string]*domain.User{}}
	codes := &memCodes{attemptTo: 3}
	a := NewAuth(users, fakeTokens{}, nil, nil, &countingSessions{}, codes, nil, Verification{})

	meta := SessionMeta{IP: "203.0.113.7"}
	for i := range 3 {
		_, err := a.VerifyEmail(context.Background(), "who@ever.io", "000000", meta)
		if !errors.Is(err, domain.ErrInvalidCode) {
			t.Fatalf("guess %d: got %v, want ErrInvalidCode", i+1, err)
		}
	}
	if _, err := a.VerifyEmail(context.Background(), "who@ever.io", "000000", meta); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("over the cap: got %v, want ErrRateLimited", err)
	}
}

func TestResetPasswordActivatesPendingAndDropsSessions(t *testing.T) {
	u := &domain.User{ID: 9, Email: "c@d.io", Status: domain.StatusPending}
	users := memUsers{
		byEmail: map[string]*domain.User{"c@d.io": u},
		pwSet:   map[int64]string{},
	}
	sessions := &countingSessions{}
	codes := &memCodes{resetFor: 9, hasReset: true}
	a := NewAuth(users, fakeTokens{}, staticHasher{out: "new-hash"}, nil, sessions, codes, nil, Verification{})

	if err := a.ResetPassword(context.Background(), "tok", "brand-new-pass"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if users.pwSet[9] != "new-hash" {
		t.Fatalf("password hash = %q, want new-hash", users.pwSet[9])
	}
	// Following the emailed link proves the address, same as the signup code.
	if u.Status != domain.StatusActive {
		t.Fatalf("status = %q, want active", u.Status)
	}
	if sessions.revokedAll != 1 {
		t.Fatalf("RevokeAll called %d times, want 1", sessions.revokedAll)
	}

	// Single use: the same link must not reset the password twice.
	if err := a.ResetPassword(context.Background(), "tok", "another-pass"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("reused token: got %v, want ErrInvalidToken", err)
	}
}

func TestLoginCapsFailuresPerSourceAndClearsOnSuccess(t *testing.T) {
	stored := "$2a$12$storedhashforanexistinguser"
	active := &domain.User{ID: 4, Status: domain.StatusActive, PasswordHash: &stored}
	calls := 0
	codes := &memCodes{attemptTo: 3}
	a := NewAuth(
		fakeUsers{user: active},
		fakeTokens{},
		countingHasher{calls: &calls, accept: "right"},
		nil, &countingSessions{}, codes, nil, Verification{},
	)
	meta := SessionMeta{IP: "198.51.100.9"}

	for i := range 3 {
		if _, err := a.Login(context.Background(), LoginInput{Login: "x", Password: "wrong"}, meta); !errors.Is(err, domain.ErrCredentials) {
			t.Fatalf("guess %d: got %v, want ErrCredentials", i+1, err)
		}
	}
	if _, err := a.Login(context.Background(), LoginInput{Login: "x", Password: "wrong"}, meta); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("over the cap: got %v, want ErrRateLimited", err)
	}

	// Nothing here should burn a bcrypt comparison once the cap has tripped:
	// the check happens before the password is looked at.
	if calls != 3 {
		t.Fatalf("bcrypt comparisons = %d, want 3 — the rate limit must short-circuit", calls)
	}

	// A success wipes the counter, so a shared address is not walked into a
	// lockout by people who all get in.
	codes.attempts = 0
	if _, err := a.Login(context.Background(), LoginInput{Login: "x", Password: "right"}, meta); err != nil {
		t.Fatalf("successful login: %v", err)
	}
	if codes.cleared != 1 {
		t.Fatalf("ClearAttempts called %d times, want 1", codes.cleared)
	}
}

func TestLoginRejectsUnverifiedAccount(t *testing.T) {
	stored := "$2a$12$storedhashforanexistinguser"
	pending := &domain.User{ID: 3, Status: domain.StatusPending, PasswordHash: &stored}
	calls := 0
	a := NewAuth(
		fakeUsers{user: pending},
		fakeTokens{},
		countingHasher{calls: &calls, accept: "right-password"},
		nil, &countingSessions{}, &memCodes{}, nil, Verification{},
	)

	_, err := a.Login(context.Background(), LoginInput{Login: "x", Password: "right-password"}, SessionMeta{})
	if !errors.Is(err, domain.ErrNotVerified) {
		t.Fatalf("got %v, want ErrNotVerified", err)
	}
}
