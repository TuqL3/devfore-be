package mail

import (
	"strings"
	"testing"
)

// The envelope and the From: header are not the same string. Sending the
// display-name form as the envelope is what earns a "501 invalid FROM
// parameter" from the server.
func TestEnvelopeStripsDisplayName(t *testing.T) {
	cases := map[string]string{
		"DevForge <no-reply@devforge.local>": "no-reply@devforge.local",
		"no-reply@devforge.local":            "no-reply@devforge.local",
		"nonsense":                           "nonsense", // passed through, server decides
	}
	for in, want := range cases {
		if got := envelopeOf(in); got != want {
			t.Errorf("envelopeOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildEncodesSubjectAndRejectsInjection(t *testing.T) {
	msg, err := build("a@b.io", "c@d.io", "Mã xác thực DevForge", "<p>hi</p>")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	s := string(msg)
	// A raw UTF-8 subject renders as mojibake in most clients.
	if !strings.Contains(s, "Subject: =?utf-8?q?") {
		t.Errorf("subject not RFC 2047 encoded:\n%s", s)
	}
	if !strings.Contains(s, "\r\n\r\n<p>hi</p>") {
		t.Errorf("body not separated from headers:\n%s", s)
	}

	if _, err := build("a@b.io", "c@d.io\r\nBcc: evil@x.io", "s", "b"); err == nil {
		t.Error("newline in recipient accepted, want rejection")
	}
}
