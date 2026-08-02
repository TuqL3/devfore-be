package chat

import (
	"sync"
)

// outbox is how many messages may be queued for one connection before it is
// considered too slow to keep. Small on purpose: a client that cannot keep up
// with a chat room is a client on a dead network, and buffering more for it just
// moves the memory leak later.
const outbox = 32

// Hub is every open connection in the room.
//
// A map in one process, which is the whole design and also its limit: run two
// API instances and each has its own room, so a message sent to one is never
// seen on the other. History still works — that is in postgres — so the symptom
// is "messages appear on refresh but not live", which is confusing rather than
// broken.
//
// ponytail: single instance until the deploy actually has two. The fix when it
// does is redis pub/sub between hubs, which slots in behind Broadcast without
// touching any caller.
type Hub struct {
	mu      sync.RWMutex
	clients map[chan Message]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan Message]struct{})}
}

// Join registers a connection and hands back its queue plus the function that
// removes it. The caller must call leave, or the hub keeps writing to a channel
// nobody reads and every broadcast pays for it.
func (h *Hub) Join() (<-chan Message, func()) {
	ch := make(chan Message, outbox)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.clients, ch)
			h.mu.Unlock()
			// Closed only after removal, so Broadcast can never select on a
			// channel that is already closed.
			close(ch)
		})
	}
}

// Broadcast fans a message out to everyone. A client whose queue is full is
// skipped rather than waited for: one suspended laptop must not stall the room
// for everybody else, and the skipped client will refetch history when it
// reconnects.
func (h *Hub) Broadcast(m Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- m:
		default:
		}
	}
}

// Count is how many connections are open. Used by the test, and by anything that
// wants to know whether the room is empty.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
