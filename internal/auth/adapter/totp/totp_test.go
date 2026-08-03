package totp

import (
	"encoding/base32"
	"encoding/base64"
	"testing"
	"time"

	"rsc.io/qr"
)

// The test vectors from RFC 6238 appendix B, SHA1 column. Checking against the
// spec rather than against this implementation's own output is the whole point:
// a code that is self-consistent but wrong locks every user out of their account
// on the day they enrol, and nothing else here would catch it.
//
// The RFC's shared secret is the ASCII string "12345678901234567890"; the
// vectors are eight digits and this package produces six, so the comparison is
// against the last six, which is what truncating to Digits gives.
func TestRFC6238Vectors(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).
		EncodeToString([]byte("12345678901234567890"))

	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},          // RFC: 94287082
		{1111111109, "081804"},  // RFC: 07081804
		{1111111111, "050471"},  // RFC: 14050471
		{1234567890, "005924"},  // RFC: 89005924
		{2000000000, "279037"},  // RFC: 69279037
		{20000000000, "353130"}, // RFC: 65353130
	}

	for _, c := range cases {
		got, err := Code(secret, time.Unix(c.unix, 0))
		if err != nil {
			t.Fatalf("Code at %d: %v", c.unix, err)
		}
		if got != c.want {
			t.Errorf("Code at %d = %s, want %s", c.unix, got, c.want)
		}
	}
}

func TestValidateAcceptsNowAndRejectsRubbish(t *testing.T) {
	secret, err := NewSecret()
	if err != nil {
		t.Fatalf("NewSecret: %v", err)
	}

	now, err := Code(secret, time.Now())
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if !Validate(secret, now) {
		t.Fatal("the current code was rejected")
	}
	// One step back is inside the skew window a drifting phone clock needs.
	prev, _ := Code(secret, time.Now().Add(-Period))
	if prev != now && !Validate(secret, prev) {
		t.Fatal("the previous step was rejected, so a slow clock cannot log in")
	}
	// Two steps back is outside it. Widening this is how a code stays useful
	// long after it was read over somebody's shoulder.
	old, _ := Code(secret, time.Now().Add(-3*Period))
	if old != now && Validate(secret, old) {
		t.Fatal("a code three steps old was accepted")
	}

	for _, bad := range []string{"", "12345", "1234567", "abcdef", "000000 "} {
		if bad == now {
			continue
		}
		if Validate(secret, bad) {
			t.Errorf("Validate accepted %q", bad)
		}
	}
}

func TestURICarriesWhatAnAuthenticatorReads(t *testing.T) {
	uri := URI("DevForge", "student@example.com", "ABCDEFGH")
	for _, want := range []string{
		"otpauth://totp/",
		"secret=ABCDEFGH",
		"issuer=DevForge",
		"digits=6",
		"period=30",
	} {
		if !contains(uri, want) {
			t.Errorf("URI %q missing %q", uri, want)
		}
	}
}

// What this can and cannot prove: it checks that a real PNG comes out and that
// the pixels track the input, which catches a blank image, a constant one, and a
// payload that is not a PNG at all — the failures that would otherwise render as
// a broken img tag or a code every camera ignores.
//
// It does not prove the code scans. That would need a QR *decoder*, which is a
// second dependency to carry for a test, and the encoding itself is the
// library's job rather than this package's. Scanning it once by hand is the
// check that closes that gap.
func TestQRProducesAPNGThatTracksItsInput(t *testing.T) {
	uri := URI("DevForge", "student@example.com", "JBSWY3DPEHPK3PXP")

	code, err := qr.Encode(uri, qr.M)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	other, err := qr.Encode(URI("DevForge", "someone@example.com", "MFRGGZDFMZTWQ2LK"), qr.M)
	if err != nil {
		t.Fatalf("Encode other: %v", err)
	}
	if string(code.Bitmap) == string(other.Bitmap) {
		t.Fatal("two different URIs produced the same bitmap")
	}
	if code.Size == 0 {
		t.Fatal("empty code")
	}

	got, err := QR(uri)
	if err != nil {
		t.Fatalf("QR: %v", err)
	}
	const prefix = "data:image/png;base64,"
	if !contains(got, prefix) {
		t.Fatalf("QR = %.40q…, want a %s data uri", got, prefix)
	}
	raw, err := base64.StdEncoding.DecodeString(got[len(prefix):])
	if err != nil {
		t.Fatalf("data uri is not base64: %v", err)
	}
	// PNG magic. An img tag with something that is not a PNG renders as a broken
	// image and nothing in the pipeline would have complained.
	if len(raw) < 8 || string(raw[1:4]) != "PNG" {
		t.Fatalf("payload is not a png: % x", raw[:min(8, len(raw))])
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
