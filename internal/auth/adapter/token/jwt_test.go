package token

import (
	"testing"
	"time"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	ts := NewJWT("test-secret", 15*time.Minute)
	access, ttl, err := ts.IssueAccess(42, []string{"student"}, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if ttl != 15*time.Minute {
		t.Fatalf("ttl = %v, want 15m", ttl)
	}

	id, roles, sid, err := ts.ParseAccess(access)
	if err != nil {
		t.Fatalf("parse access: %v", err)
	}
	if id != 42 {
		t.Fatalf("id = %d, want 42", id)
	}
	if len(roles) != 1 || roles[0] != "student" {
		t.Fatalf("roles = %v", roles)
	}
	if sid != "sess-1" {
		t.Fatalf("session id = %q, want sess-1", sid)
	}

	other := NewJWT("other-secret", time.Minute)
	if _, _, _, err := other.ParseAccess(access); err == nil {
		t.Fatal("token accepted with wrong secret")
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	ts := NewJWT("s", -time.Second)
	access, _, _ := ts.IssueAccess(1, nil, "s1")
	if _, _, _, err := ts.ParseAccess(access); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestForeignTokenTypeRejected(t *testing.T) {
	ts := NewJWT("s", time.Minute)
	foreign, err := ts.sign(1, "something-else", nil, "s1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ts.ParseAccess(foreign); err == nil {
		t.Fatal("foreign token type accepted as access")
	}
}
