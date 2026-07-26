package token

import (
	"testing"
	"time"
)

func TestTokenRoundTrip(t *testing.T) {
	ts := NewJWT("test-secret", 15*time.Minute, time.Hour)
	tp, err := ts.Issue(42, []string{"student"})
	if err != nil {
		t.Fatal(err)
	}

	id, roles, err := ts.ParseAccess(tp.AccessToken)
	if err != nil {
		t.Fatalf("parse access: %v", err)
	}
	if id != 42 {
		t.Fatalf("id = %d, want 42", id)
	}
	if len(roles) != 1 || roles[0] != "student" {
		t.Fatalf("roles = %v", roles)
	}

	if _, err := ts.ParseRefresh(tp.AccessToken); err == nil {
		t.Fatal("access token accepted as refresh")
	}
	if _, _, err := ts.ParseAccess(tp.RefreshToken); err == nil {
		t.Fatal("refresh token accepted as access")
	}

	other := NewJWT("other-secret", time.Minute, time.Hour)
	if _, _, err := other.ParseAccess(tp.AccessToken); err == nil {
		t.Fatal("token accepted with wrong secret")
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	ts := NewJWT("s", -time.Second, time.Hour)
	tp, _ := ts.Issue(1, nil)
	if _, _, err := ts.ParseAccess(tp.AccessToken); err == nil {
		t.Fatal("expired token accepted")
	}
}
