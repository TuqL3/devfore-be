package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/devforge/be/internal/auth/domain"
)

type fakeUsers struct {
	UserRepository
	user *domain.User
	err  error
}

func (f fakeUsers) ByLogin(context.Context, string) (*domain.User, error) { return f.user, f.err }

type countingHasher struct {
	PasswordHasher
	calls  *int
	accept string
}

func (h countingHasher) Check(hash, plain string) bool {
	*h.calls++
	return hash != placeholderHash && plain == h.accept
}

type fakeSessions struct{ SessionStore }

func (fakeSessions) Create(_ context.Context, uid int64, _ SessionMeta) (domain.Session, error) {
	return domain.Session{ID: "sid-1", UserID: uid}, nil
}

func (fakeSessions) TTL() time.Duration { return time.Hour }

type fakeTokens struct{ TokenIssuer }

func (fakeTokens) IssueAccess(int64, []string, string) (string, time.Duration, error) {
	return "access-token", time.Minute, nil
}

func TestLoginHashesOnceOnEveryPath(t *testing.T) {
	stored := "$2a$12$storedhashforanexistinguser"
	active := &domain.User{ID: 7, Status: domain.StatusActive, PasswordHash: &stored}
	googleOnly := &domain.User{ID: 8, Status: domain.StatusActive}
	banned := &domain.User{ID: 9, Status: domain.StatusBanned, PasswordHash: &stored}

	tests := []struct {
		name     string
		user     *domain.User
		repoErr  error
		password string
		wantErr  error
	}{
		{"correct password", active, nil, "right", nil},
		{"wrong password", active, nil, "wrong", domain.ErrCredentials},
		{"unknown login", nil, domain.ErrNotFound, "right", domain.ErrCredentials},
		{"google-only account", googleOnly, nil, "right", domain.ErrCredentials},
		{"banned user", banned, nil, "right", domain.ErrBanned},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			a := NewAuth(
				fakeUsers{user: tt.user, err: tt.repoErr},
				fakeTokens{},
				countingHasher{calls: &calls, accept: "right"},
				nil,
				fakeSessions{}, nil, nil, Verification{},
			)

			_, err := a.Login(context.Background(),
				LoginInput{Login: "Someone", Password: tt.password}, SessionMeta{})

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
			if calls != 1 {
				t.Errorf("bcrypt comparisons = %d, want exactly 1", calls)
			}
		})
	}
}

func TestLoginPropagatesRepoError(t *testing.T) {
	boom := errors.New("connection refused")
	calls := 0
	a := NewAuth(
		fakeUsers{err: boom},
		fakeTokens{},
		countingHasher{calls: &calls, accept: "right"},
		nil,
		fakeSessions{}, nil, nil, Verification{},
	)

	if _, err := a.Login(context.Background(), LoginInput{Login: "x", Password: "y"}, SessionMeta{}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}
