package hash

import "testing"

func TestPasswordHash(t *testing.T) {
	var h Bcrypt
	hashed, err := h.Hash("s3cr3tpass")
	if err != nil {
		t.Fatal(err)
	}
	if !h.Check(hashed, "s3cr3tpass") {
		t.Fatal("correct password rejected")
	}
	if h.Check(hashed, "wrong") {
		t.Fatal("wrong password accepted")
	}
}
