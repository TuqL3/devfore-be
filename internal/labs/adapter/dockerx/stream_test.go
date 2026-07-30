package dockerx

import (
	"strings"
	"testing"
)

// stdcopy treats a short write as a failure and stops demultiplexing, so the cap
// has to be applied while still claiming the whole slice was taken. Reporting the
// truncated length instead turns a chatty check script into an exec that errors.
func TestLimitedBufferReportsFullWriteWhileTruncating(t *testing.T) {
	var w limitedBuffer

	chunk := strings.Repeat("x", 4096)
	for range 4 { // 16KB into an 8KB cap
		n, err := w.Write([]byte(chunk))
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		if n != len(chunk) {
			t.Fatalf("short write reported: got %d, want %d", n, len(chunk))
		}
	}

	out := w.String()
	if !strings.HasSuffix(out, "… (đã cắt bớt)") {
		t.Errorf("truncation not announced, output ends: %q", out[max(0, len(out)-40):])
	}
	if body := strings.TrimSuffix(out, "\n… (đã cắt bớt)"); len(body) != checkOutputLimit {
		t.Errorf("kept %d bytes, want exactly %d", len(body), checkOutputLimit)
	}
}

func TestLimitedBufferLeavesShortOutputAlone(t *testing.T) {
	var w limitedBuffer
	if _, err := w.Write([]byte("CHECK_PASS\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := w.String(); got != "CHECK_PASS\n" {
		t.Errorf("got %q, want %q", got, "CHECK_PASS\n")
	}
}

// The session id goes straight into a container name, and docker only accepts
// [a-zA-Z0-9][a-zA-Z0-9_.-]* there. Base64 url encoding stays inside that set,
// which is the reason the ids are generated that way rather than with padding.
func TestContainerNameIsAValidDockerName(t *testing.T) {
	const valid = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.-"
	name := ContainerName("89Uqujd5XBBAWbc12mcuzg")
	if !strings.HasPrefix(name, "devforge-lab-") {
		t.Fatalf("prefix missing: %q", name)
	}
	for i, r := range name {
		if !strings.ContainsRune(valid, r) {
			t.Errorf("byte %d of %q is %q, which docker rejects", i, name, r)
		}
	}
}
