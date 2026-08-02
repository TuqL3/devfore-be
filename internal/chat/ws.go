package chat

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	// The browser has to prove it is still there, same as the lab terminal: a
	// suspended laptop should stop counting as somebody in the room.
	pongWait  = 60 * time.Second
	pingEvery = 25 * time.Second
	writeWait = 10 * time.Second
	// Generous next to MaxBody, which is measured in runes and enforced after
	// decoding. This one only stops a frame big enough to be an attack.
	readLimit = 8 << 10
)

// Rate limit, per connection. A chat message is cheap to send and cheap to
// store, so the thing being defended is the room's readability rather than the
// server: burst lets a person paste three lines in a row, the refill stops a
// script.
const (
	burst      = 5
	refillTick = 2 * time.Second
)

type incoming struct {
	Body string `json:"body"`
}

// Handle upgrades one connection and runs it until the client goes away, the
// server shuts down, or the session it was authorised by would have expired.
//
// That last one is the interesting bound. The handshake is authorised once, by
// the cookie the middleware already checked, and nothing re-checks it while the
// socket stays open — so a user banned mid-conversation could otherwise keep
// talking indefinitely. Capping the connection at one access-token lifetime
// bounds that to minutes: the reconnect that follows has to pass the middleware
// again, and a banned account no longer has a session to pass it with.
func (m *Module) Handle(c *gin.Context) {
	userID := m.userID(c)
	username := m.username(c)
	if userID == 0 || username == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "chưa đăng nhập"})
		return
	}

	ws, err := m.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Upgrade has written its own response by now.
		slog.Warn("chat upgrade", "user", userID, "err", err)
		return
	}
	defer ws.Close()

	// Outlives the request, which gin cancels the moment this handler returns.
	ctx, cancel := context.WithTimeout(
		context.WithoutCancel(c.Request.Context()), m.sessionWindow)
	defer cancel()

	feed, leave := m.hub.Join()
	defer leave()

	go m.writeLoop(ctx, ws, feed)
	m.readLoop(ctx, ws, userID, username)
}

// readLoop owns the connection's read side and returns when it ends. Everything
// that writes to the socket lives in writeLoop instead: gorilla allows one
// concurrent writer, and a ping racing a broadcast is a corrupted frame.
func (m *Module) readLoop(ctx context.Context, ws *websocket.Conn, userID int64, username string) {
	ws.SetReadLimit(readLimit)
	_ = ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	// Token bucket computed from elapsed time rather than refilled by a ticker.
	// A goroutine topping this up would be writing what this loop reads, which
	// is a data race for the sake of avoiding one subtraction.
	tokens := float64(burst)
	last := time.Now()

	for {
		if ctx.Err() != nil {
			return
		}
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return
		}

		var in incoming
		if err := json.Unmarshal(raw, &in); err != nil {
			continue
		}
		body, ok := Clean(in.Body)
		if !ok {
			continue
		}
		now := time.Now()
		tokens = min(burst, tokens+now.Sub(last).Seconds()/refillTick.Seconds())
		last = now
		// Dropped silently rather than answered with an error: a client that
		// respects the limit never sees this, and one that does not is a script
		// that will not read the reply either.
		if tokens < 1 {
			continue
		}
		tokens--

		msg := Message{UserID: &userID, Username: username, Body: body}
		// Stored before it is sent. The database assigns the id and the
		// timestamp, so every client orders the room the same way — and a
		// message that failed to store is never shown to anyone, rather than
		// appearing live and vanishing on the next reload.
		if err := m.repo.Save(ctx, &msg); err != nil {
			slog.Error("chat save", "user", userID, "err", err)
			continue
		}
		m.hub.Broadcast(msg)
	}
}

// writeLoop owns the write side: broadcasts, pings, and the close when the
// authorised window runs out.
func (m *Module) writeLoop(ctx context.Context, ws *websocket.Conn, feed <-chan Message) {
	ping := time.NewTicker(pingEvery)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			// Tells the client this was a deadline rather than a fault, so it
			// reconnects instead of showing an error.
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			_ = ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "phiên hết hạn"),
				time.Now().Add(writeWait))
			_ = ws.Close()
			return

		case msg, ok := <-feed:
			if !ok {
				return
			}
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteJSON(msg); err != nil {
				_ = ws.Close()
				return
			}

		case <-ping.C:
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				_ = ws.Close()
				return
			}
		}
	}
}

// History is what a joining client reads before the socket opens. A plain GET
// rather than a first websocket frame: it is cacheable, it works when the
// upgrade fails, and it keeps the socket protocol to one message shape.
func (m *Module) History(c *gin.Context) {
	msgs, err := m.repo.Recent(c.Request.Context())
	if err != nil {
		slog.Error("chat history", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "lỗi máy chủ"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs, "online": m.hub.Count()})
}

func originAllowed(allowed []string) func(*http.Request) bool {
	// A websocket handshake carries cookies like any other request and the
	// same-origin policy does not apply to it, so without this any page on the
	// internet could open one as a logged-in student and post as them.
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		// No Origin means not a browser, which is not the attack this guards
		// against — the session cookie still has to be valid.
		return origin == "" || slices.Contains(allowed, origin)
	}
}
