package chat

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// The hub holds a channel per open connection, so the two ways it can go wrong
// are both quiet: a client that leaves without being removed keeps every future
// broadcast paying for it, and a client that stops reading blocks the room for
// everybody if Broadcast waits on it.
func TestBroadcastSkipsTheSlowAndForgetsTheGone(t *testing.T) {
	h := NewHub()

	fast, leaveFast := h.Join()
	_, leaveSlow := h.Join() // never read from
	if h.Count() != 2 {
		t.Fatalf("count = %d, want 2", h.Count())
	}

	// More than one client's queue can hold. If Broadcast waited on the reader
	// that never reads, this would not return and the test would time out.
	done := make(chan struct{})
	go func() {
		for i := range outbox * 3 {
			h.Broadcast(Message{ID: int64(i), Body: "x"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Broadcast blocked on a client that is not reading")
	}

	// The reader that is keeping up still got messages, in order.
	first := <-fast
	second := <-fast
	if second.ID <= first.ID {
		t.Fatalf("out of order: %d then %d", first.ID, second.ID)
	}

	leaveSlow()
	if h.Count() != 1 {
		t.Fatalf("count after leave = %d, want 1", h.Count())
	}

	// Leaving twice is what a deferred leave plus an error path looks like. It
	// must not panic on a channel that is already closed.
	leaveSlow()
	leaveFast()
	leaveFast()
	if h.Count() != 0 {
		t.Fatalf("count after all left = %d, want 0", h.Count())
	}
}

// Join and Broadcast run on different goroutines for every connection, so the
// map underneath has to be safe. Run with -race to mean anything.
func TestHubIsSafeUnderConcurrentJoinAndBroadcast(t *testing.T) {
	h := NewHub()
	var wg sync.WaitGroup

	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, leave := h.Join()
			defer leave()
			go func() {
				for range ch {
				}
			}()
			time.Sleep(time.Millisecond)
		}()
	}
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Broadcast(Message{Body: "hi"})
		}()
	}
	wg.Wait()
}

func TestCleanRejectsEmptyAndTruncatesLong(t *testing.T) {
	for _, empty := range []string{"", "   ", "\n\t "} {
		if _, ok := Clean(empty); ok {
			t.Errorf("Clean(%q) accepted an empty message", empty)
		}
	}

	if got, ok := Clean("  xin chào  "); !ok || got != "xin chào" {
		t.Fatalf("Clean trimmed to %q, ok=%v", got, ok)
	}

	// Counted in runes, not bytes: a Vietnamese message must not be cut shorter
	// than an English one of the same length.
	long := strings.Repeat("ế", MaxBody+50)
	got, ok := Clean(long)
	if !ok {
		t.Fatal("Clean rejected a long message instead of truncating it")
	}
	if n := len([]rune(got)); n != MaxBody {
		t.Fatalf("truncated to %d runes, want %d", n, MaxBody)
	}
}
