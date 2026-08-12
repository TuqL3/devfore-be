package chat

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/devforge/be/internal/i18n"
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

// What a client may ask for over the socket. One shape with a kind rather than
// three endpoints: editing and deleting have to reach the same audience the
// message did, and that audience is only known here.
type incoming struct {
	Kind string `json:"kind"` // "send" | "edit" | "delete"
	// Recipient of a direct message. Absent or zero means the shared room.
	PeerID int64  `json:"peer_id"`
	ID     int64  `json:"id"` // edit/delete target
	Body   string `json:"body"`
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
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			gin.H{"error": i18n.Msg(c, "chưa đăng nhập")})
		return
	}

	// Captured before the upgrade: writeLoop outlives the request.
	lang := i18n.From(c)

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

	feed, leave := m.hub.Join(userID)
	defer leave()

	go m.writeLoop(ctx, ws, feed, lang)
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

		switch in.Kind {
		case "edit":
			m.handleEdit(ctx, in, userID)
		case "delete":
			m.handleDelete(ctx, in, userID)
		default:
			m.handleSend(ctx, in, userID, username)
		}
	}
}

func (m *Module) handleSend(ctx context.Context, in incoming, userID int64, username string) {
	body, ok := Clean(in.Body)
	if !ok {
		return
	}

	msg := Message{UserID: &userID, Username: username, Body: body}
	if in.PeerID != 0 {
		// Checked rather than trusted: the peer id comes off the wire, and
		// without this a hand-written frame could open a thread with a banned
		// or deleted account — or with the sender themselves, which the schema
		// refuses and would surface as a constraint error instead of a reason.
		ok, err := m.repo.CanReceive(ctx, in.PeerID)
		if err != nil {
			slog.Error("chat peer check", "user", userID, "err", err)
			return
		}
		if !ok || in.PeerID == userID {
			return
		}
		peer := in.PeerID
		msg.PeerID = &peer
	}

	// Stored before it is sent. The database assigns the id and the timestamp,
	// so every client orders the thread the same way — and a message that
	// failed to store is never shown to anyone, rather than appearing live and
	// vanishing on the next reload.
	if err := m.repo.Save(ctx, &msg); err != nil {
		slog.Error("chat save", "user", userID, "err", err)
		return
	}
	m.hub.Broadcast(Event{Kind: "message", Message: msg})
}

func (m *Module) handleEdit(ctx context.Context, in incoming, userID int64) {
	body, ok := Clean(in.Body)
	if !ok || in.ID == 0 {
		return
	}
	msg, err := m.repo.Edit(ctx, in.ID, userID, body)
	if err != nil {
		// Not theirs, already deleted, or gone. None of the three is worth
		// telling the sender apart — a client that edits what it did not write
		// is not a client with a user behind it.
		if !errors.Is(err, ErrNotAllowed) && !errors.Is(err, ErrNotFound) {
			slog.Error("chat edit", "user", userID, "id", in.ID, "err", err)
		}
		return
	}
	m.hub.Broadcast(Event{Kind: "update", Message: *msg})
}

func (m *Module) handleDelete(ctx context.Context, in incoming, userID int64) {
	if in.ID == 0 {
		return
	}
	msg, err := m.repo.Delete(ctx, in.ID, userID)
	if err != nil {
		if !errors.Is(err, ErrNotAllowed) && !errors.Is(err, ErrNotFound) {
			slog.Error("chat delete", "user", userID, "id", in.ID, "err", err)
		}
		return
	}
	m.hub.Broadcast(Event{Kind: "update", Message: *msg})
}

// writeLoop owns the write side: broadcasts, pings, and the close when the
// authorised window runs out.
func (m *Module) writeLoop(ctx context.Context, ws *websocket.Conn, feed <-chan Event, lang i18n.Lang) {
	ping := time.NewTicker(pingEvery)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			// Tells the client this was a deadline rather than a fault, so it
			// reconnects instead of showing an error.
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			_ = ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure,
					i18n.Translate(lang, "phiên hết hạn")),
				time.Now().Add(writeWait))
			_ = ws.Close()
			return

		case e, ok := <-feed:
			if !ok {
				return
			}
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteJSON(e); err != nil {
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

// Messages is the tail a client reads before the socket opens: the shared room
// by default, or one thread when ?peer= names somebody. A plain GET rather than
// a first websocket frame — it is cacheable, it works when the upgrade fails,
// and it keeps the socket protocol to one shape.
func (m *Module) Messages(c *gin.Context) {
	me := m.userID(c)

	// Reading further back. Garbage is the newest page rather than a 400: the
	// cursor is a scroll position, and refusing one leaves the screen stuck.
	before, _ := strconv.ParseInt(c.Query("before"), 10, 64)
	if before < 0 {
		before = 0
	}

	var (
		msgs []Message
		err  error
	)
	if raw := c.Query("peer"); raw != "" {
		peer, convErr := strconv.ParseInt(raw, 10, 64)
		if convErr != nil || peer <= 0 || peer == me {
			c.AbortWithStatusJSON(http.StatusBadRequest,
				gin.H{"error": i18n.Msg(c, "peer không hợp lệ")})
			return
		}
		// Scoped to the caller's own pair. There is no id a client can send
		// that reads somebody else's thread, because the caller is always one
		// half of the key.
		msgs, err = m.repo.Thread(c.Request.Context(), me, peer, before)
	} else {
		msgs, err = m.repo.Room(c.Request.Context(), before)
	}
	if err != nil {
		slog.Error("chat history", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": i18n.Msg(c, "lỗi máy chủ")})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs, "online": m.hub.Count()})
}

// Conversations is the sidebar: every direct thread the caller has, newest
// first. The shared room is not in it — it always exists, and a row saying so
// would be a row that can go missing.
func (m *Module) Conversations(c *gin.Context) {
	out, err := m.repo.Conversations(c.Request.Context(), m.userID(c))
	if err != nil {
		slog.Error("chat conversations", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": i18n.Msg(c, "lỗi máy chủ")})
		return
	}
	c.JSON(http.StatusOK, gin.H{"conversations": out})
}

// People backs "message someone". Signed-in only, active accounts only, and
// never the caller.
func (m *Module) People(c *gin.Context) {
	out, err := m.repo.People(c.Request.Context(), m.userID(c), c.Query("q"))
	if err != nil {
		slog.Error("chat people", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": i18n.Msg(c, "lỗi máy chủ")})
		return
	}
	c.JSON(http.StatusOK, gin.H{"people": out})
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
