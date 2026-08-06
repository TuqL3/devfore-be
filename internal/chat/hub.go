package chat

import (
	"sync"
)

// outbox is how many events may be queued for one connection before it is
// considered too slow to keep. Small on purpose: a client that cannot keep up
// with a chat room is a client on a dead network, and buffering more for it just
// moves the memory leak later.
const outbox = 32

// Event is what goes over the wire to a client. Kind separates a new message
// from one that changed, so the screen knows whether to append or replace.
type Event struct {
	Kind    string  `json:"kind"` // "message" | "update"
	Message Message `json:"message"`
}

type client struct {
	userID int64
	ch     chan Event
}

// Hub is every open connection, and who is behind each one.
//
// Knowing the user id is what makes a direct thread possible: a room event goes
// to everyone, a thread event goes to exactly the two people in it. Getting that
// wrong is not a cosmetic bug — it is delivering somebody's private message to
// the whole platform, which is why Audience is a separate, tested function
// rather than an if buried in the loop.
//
// A map in one process, which is the design and also its limit: run two API
// instances and each has its own set of rooms, so a message sent to one is never
// seen on the other. History still works — that is in postgres — so the symptom
// is "messages appear on refresh but not live".
//
// ponytail: single instance until the deploy actually has two. The fix when it
// does is redis pub/sub between hubs, which slots in behind Broadcast without
// touching any caller.
type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

// Join registers a connection and hands back its queue plus the function that
// removes it. The caller must call leave, or the hub keeps writing to a channel
// nobody reads and every broadcast pays for it.
func (h *Hub) Join(userID int64) (<-chan Event, func()) {
	c := &client{userID: userID, ch: make(chan Event, outbox)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	return c.ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.clients, c)
			h.mu.Unlock()
			// Closed only after removal, so Broadcast can never select on a
			// channel that is already closed.
			close(c.ch)
		})
	}
}

// Audience reports whether a connection belonging to viewer should receive a
// message. A room message reaches everyone; a direct one reaches only its two
// ends, whichever direction it went.
//
// Its own function because it is the rule that keeps private messages private,
// and a rule like that should be readable on its own and testable without a
// socket.
func Audience(m Message, viewer int64) bool {
	if m.PeerID == nil {
		return true
	}
	if *m.PeerID == viewer {
		return true
	}
	return m.UserID != nil && *m.UserID == viewer
}

// Broadcast fans an event out to everyone it belongs to. A client whose queue is
// full is skipped rather than waited for: one suspended laptop must not stall
// the room for everybody else, and the skipped client refetches history when it
// reconnects.
func (h *Hub) Broadcast(e Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if !Audience(e.Message, c.userID) {
			continue
		}
		select {
		case c.ch <- e:
		default:
		}
	}
}

// Count is how many connections are open.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
