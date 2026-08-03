package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// End to end over a real socket against a real database, because the parts that
// can break are the seams: a message that broadcasts but never persists, one
// that persists but never reaches the other person in the room, or a name that
// comes back as somebody else's.
func TestRoomDeliversAndPersists(t *testing.T) {
	db := openDB(t)
	gin.SetMode(gin.TestMode)

	alice, aliceName := seedUser(t, db)
	bob, bobName := seedUser(t, db)

	m := New(db, Config{SessionWindow: time.Minute})
	srv, dial := serve(t, m)
	defer srv.Close()

	aliceWS := dial(t, alice)
	defer aliceWS.Close()
	bobWS := dial(t, bob)
	defer bobWS.Close()

	// Both sockets have to be registered before anything is sent, or the test
	// races the hub rather than testing it.
	waitFor(t, func() bool { return m.hub.Count() == 2 })

	body := "xin chào phòng " + fmt.Sprint(time.Now().UnixNano())
	send(t, aliceWS, body)

	// The sender sees it too: the room is one list, not "mine plus theirs".
	ev := read(t, aliceWS)
	got := ev.Message
	if got.Body != body || got.Username != aliceName {
		t.Fatalf("sender got %+v, want body %q from %q", got, body, aliceName)
	}
	if got.ID == 0 || got.CreatedAt.IsZero() {
		t.Fatalf("message has no id or timestamp: %+v — the database has to assign both", got)
	}

	onBob := read(t, bobWS).Message
	if onBob.ID != got.ID || onBob.Body != body {
		t.Fatalf("bob got %+v, want the same message alice sent", onBob)
	}

	// Persisted, so somebody joining later reads it.
	history, err := m.repo.Room(context.Background(), 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) == 0 || history[len(history)-1].ID != got.ID {
		t.Fatalf("last stored message is not the one just sent")
	}

	// Whitespace-only never reaches the room. Proved by sending a real message
	// after it and seeing that one arrive next — if the blank had gone through,
	// this read would return it instead.
	send(t, bobWS, "   \n\t ")
	send(t, bobWS, "thật")
	next := read(t, aliceWS).Message
	if next.Body != "thật" || next.Username != bobName {
		t.Fatalf("got %+v, want the blank dropped and %q from %q", next, "thật", bobName)
	}
}

// A message longer than the limit is truncated rather than refused, and the
// truncation is counted in runes — a Vietnamese sentence must not be cut shorter
// than an English one of the same length.
func TestLongMessageIsTruncatedNotDropped(t *testing.T) {
	db := openDB(t)
	gin.SetMode(gin.TestMode)
	user, _ := seedUser(t, db)

	m := New(db, Config{SessionWindow: time.Minute})
	srv, dial := serve(t, m)
	defer srv.Close()

	ws := dial(t, user)
	defer ws.Close()
	waitFor(t, func() bool { return m.hub.Count() == 1 })

	send(t, ws, strings.Repeat("ế", MaxBody+100))
	got := read(t, ws).Message
	if n := len([]rune(got.Body)); n != MaxBody {
		t.Fatalf("stored %d runes, want %d", n, MaxBody)
	}
}

// The bucket allows a short burst and then throttles. Without it one script
// makes the room unreadable for everybody.
func TestBurstIsCappedPerConnection(t *testing.T) {
	db := openDB(t)
	gin.SetMode(gin.TestMode)
	user, _ := seedUser(t, db)

	m := New(db, Config{SessionWindow: time.Minute})
	srv, dial := serve(t, m)
	defer srv.Close()

	ws := dial(t, user)
	defer ws.Close()
	waitFor(t, func() bool { return m.hub.Count() == 1 })

	const flood = burst + 10
	for i := range flood {
		send(t, ws, fmt.Sprintf("spam %d", i))
	}

	// Read whatever arrives within a short window; the throttled ones never do.
	delivered := 0
	_ = ws.SetReadDeadline(time.Now().Add(750 * time.Millisecond))
	for {
		var e Event
		if err := ws.ReadJSON(&e); err != nil {
			break
		}
		delivered++
	}
	if delivered == 0 {
		t.Fatal("nothing got through — the limiter is rejecting everything")
	}
	if delivered >= flood {
		t.Fatalf("all %d got through, want the burst capped near %d", delivered, burst)
	}
}

// serve mounts the module behind a middleware that stands in for the real auth
// one, which is the only thing a test cannot reasonably bring along. Everything
// downstream of the context key is the real code path.
func serve(t *testing.T, m *Module) (*httptest.Server, func(*testing.T, int64) *websocket.Conn) {
	t.Helper()
	r := gin.New()
	api := r.Group("/api")
	m.Routes(r, api, func(c *gin.Context) {
		var id int64
		_, _ = fmt.Sscan(c.Query("as"), &id)
		if id == 0 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set("user_id", id)
		c.Next()
	})

	srv := httptest.NewServer(r)
	dial := func(t *testing.T, userID int64) *websocket.Conn {
		t.Helper()
		u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/chat?as=" + fmt.Sprint(userID)
		ws, resp, err := websocket.DefaultDialer.Dial(u, nil)
		if err != nil {
			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			t.Fatalf("dial (http %d): %v", status, err)
		}
		return ws
	}
	return srv, dial
}

func send(t *testing.T, ws *websocket.Conn, body string) {
	t.Helper()
	if err := ws.WriteJSON(incoming{Kind: "send", Body: body}); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func sendDM(t *testing.T, ws *websocket.Conn, peer int64, body string) {
	t.Helper()
	if err := ws.WriteJSON(incoming{Kind: "send", PeerID: peer, Body: body}); err != nil {
		t.Fatalf("write dm: %v", err)
	}
}

func ask(t *testing.T, ws *websocket.Conn, in incoming) {
	t.Helper()
	if err := ws.WriteJSON(in); err != nil {
		t.Fatalf("write %s: %v", in.Kind, err)
	}
}

func read(t *testing.T, ws *websocket.Conn) Event {
	t.Helper()
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var e Event
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return e
}

// nextIs sends a sentinel down `from` and asserts it is the very next thing to
// reach `watch`. That is how "this must not be delivered" is checked: anything
// that was wrongly broadcast is queued ahead of the sentinel and shows up here
// instead of it.
//
// Waiting for a read to time out would be the obvious way and is the wrong one —
// gorilla treats any read error, deadline included, as permanent, so the socket
// would be unusable for the rest of the test.
func nextIs(t *testing.T, watch, from *websocket.Conn, why string) {
	t.Helper()
	sentinel := fmt.Sprintf("sentinel-%d", time.Now().UnixNano())
	send(t, from, sentinel)
	got := read(t, watch).Message
	if got.Body != sentinel {
		t.Fatalf("%s — but %q arrived first", why, got.Body)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition never became true")
}

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("cần DATABASE_URL trỏ tới postgres đã migrate")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	return db
}

func seedUser(t *testing.T, db *gorm.DB) (int64, string) {
	t.Helper()
	name := fmt.Sprintf("chattest%d", time.Now().UnixNano())
	var id int64
	err := db.Raw(
		`INSERT INTO users (username, email, password_hash, status)
		 VALUES (?, ?, 'x', 'active') RETURNING id`,
		name, name+"@test.local",
	).Scan(&id).Error
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM chat_messages WHERE user_id = ?`, id)
		db.Exec(`DELETE FROM users WHERE id = ?`, id)
	})
	return id, name
}

// The rule that keeps private messages private. Tested without a socket first,
// because it is small enough to reason about and too important to only exercise
// through three goroutines and a network.
func TestAudienceKeepsDirectMessagesBetweenTwoPeople(t *testing.T) {
	id := func(n int64) *int64 { return &n }

	room := Message{UserID: id(1)}
	for _, viewer := range []int64{1, 2, 999} {
		if !Audience(room, viewer) {
			t.Fatalf("room message hidden from %d", viewer)
		}
	}

	dm := Message{UserID: id(1), PeerID: id(2)}
	if !Audience(dm, 1) {
		t.Fatal("sender cannot see their own direct message")
	}
	if !Audience(dm, 2) {
		t.Fatal("recipient cannot see the direct message")
	}
	for _, outsider := range []int64{3, 999, 0} {
		if Audience(dm, outsider) {
			t.Fatalf("direct message leaked to %d", outsider)
		}
	}
}

// The same rule over a real socket, because Audience being right is only half
// of it — the hub also has to know which user each connection belongs to.
func TestDirectMessageReachesOnlyThePair(t *testing.T) {
	db := openDB(t)
	gin.SetMode(gin.TestMode)

	alice, _ := seedUser(t, db)
	bob, bobName := seedUser(t, db)
	carol, _ := seedUser(t, db)

	m := New(db, Config{SessionWindow: time.Minute})
	srv, dial := serve(t, m)
	defer srv.Close()

	aliceWS := dial(t, alice)
	defer aliceWS.Close()
	bobWS := dial(t, bob)
	defer bobWS.Close()
	carolWS := dial(t, carol)
	defer carolWS.Close()
	waitFor(t, func() bool { return m.hub.Count() == 3 })

	sendDM(t, bobWS, alice, "chỉ hai đứa mình")

	onBob := read(t, bobWS).Message
	if onBob.PeerID == nil || *onBob.PeerID != alice {
		t.Fatalf("sender got %+v, want a direct message to alice", onBob)
	}
	onAlice := read(t, aliceWS).Message
	if onAlice.ID != onBob.ID || onAlice.Username != bobName {
		t.Fatalf("alice got %+v, want bob's message", onAlice)
	}
	// Carol is not in the thread. Her next event has to be the room sentinel,
	// which it cannot be if the direct message reached her.
	nextIs(t, carolWS, aliceWS, "carol is not in the thread")
	read(t, aliceWS) // alice's own sentinel
	read(t, bobWS)   // and bob's copy of it

	// And it is not in the shared room's history either.
	room, err := m.repo.Room(context.Background(), 0)
	if err != nil {
		t.Fatalf("room: %v", err)
	}
	for _, msg := range room {
		if msg.ID == onBob.ID {
			t.Fatal("a direct message showed up in the shared room")
		}
	}

	// The thread reads the same from both ends, whichever direction it went.
	fromAlice, err := m.repo.Thread(context.Background(), alice, bob, 0)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	fromBob, err := m.repo.Thread(context.Background(), bob, alice, 0)
	if err != nil {
		t.Fatalf("thread reversed: %v", err)
	}
	if len(fromAlice) != len(fromBob) || len(fromAlice) == 0 {
		t.Fatalf("thread is not symmetric: %d vs %d", len(fromAlice), len(fromBob))
	}
}

// Editing and deleting belong to the author and nobody else, and the change has
// to reach the same people the message did.
func TestEditAndDeleteAreTheAuthorsAlone(t *testing.T) {
	db := openDB(t)
	gin.SetMode(gin.TestMode)

	alice, _ := seedUser(t, db)
	bob, _ := seedUser(t, db)

	m := New(db, Config{SessionWindow: time.Minute})
	srv, dial := serve(t, m)
	defer srv.Close()

	aliceWS := dial(t, alice)
	defer aliceWS.Close()
	bobWS := dial(t, bob)
	defer bobWS.Close()
	waitFor(t, func() bool { return m.hub.Count() == 2 })

	send(t, aliceWS, "bản gốc")
	sent := read(t, aliceWS).Message
	read(t, bobWS) // bob sees it too

	// Bob is not the author. Nothing happens, and nothing is broadcast.
	ask(t, bobWS, incoming{Kind: "edit", ID: sent.ID, Body: "bob viết đè"})
	ask(t, bobWS, incoming{Kind: "delete", ID: sent.ID})
	nextIs(t, bobWS, aliceWS, "a non-author edit or delete must broadcast nothing")
	read(t, aliceWS) // alice's own sentinel

	after, err := m.repo.ByID(context.Background(), sent.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if after.Body != "bản gốc" || after.EditedAt != nil || after.DeletedAt != nil {
		t.Fatalf("message = %+v, want untouched by somebody who did not write it", after)
	}

	// The author can, and both ends see the update.
	ask(t, aliceWS, incoming{Kind: "edit", ID: sent.ID, Body: "đã sửa"})
	ev := read(t, aliceWS)
	if ev.Kind != "update" || ev.Message.Body != "đã sửa" || ev.Message.EditedAt == nil {
		t.Fatalf("edit event = %+v, want an update carrying edited_at", ev)
	}
	if onBob := read(t, bobWS); onBob.Message.Body != "đã sửa" {
		t.Fatalf("bob saw %+v, want the edit", onBob.Message)
	}

	// Deleting keeps the row and drops the text — the id has to survive so
	// everyone else's scroll position does not jump.
	ask(t, aliceWS, incoming{Kind: "delete", ID: sent.ID})
	gone := read(t, aliceWS).Message
	if gone.ID != sent.ID {
		t.Fatalf("delete changed the id: %d became %d", sent.ID, gone.ID)
	}
	if gone.DeletedAt == nil || gone.Body != "" {
		t.Fatalf("deleted message = %+v, want an empty body and a tombstone", gone)
	}
	read(t, bobWS)

	// Editing something already taken back would put the text back on the
	// screen of anyone who had not reloaded.
	ask(t, aliceWS, incoming{Kind: "edit", ID: sent.ID, Body: "quay lại"})
	nextIs(t, aliceWS, bobWS, "a deleted message must not be editable")
	read(t, bobWS) // bob's own sentinel
	final, err := m.repo.ByID(context.Background(), sent.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if final.Body != "" {
		t.Fatalf("deleted message came back as %q", final.Body)
	}
}

// Paging backwards through a thread: the cursor must hand back the next page
// with nothing repeated and nothing skipped, which is what a scroll upward
// stitches together.
func TestThreadPagesBackwards(t *testing.T) {
	db := openDB(t)
	alice, aliceName := seedUser(t, db)
	bob, _ := seedUser(t, db)

	repo := NewRepo(db)
	const extra = 5
	for i := 0; i < HistoryLimit+extra; i++ {
		m := &Message{UserID: &alice, Username: aliceName, PeerID: &bob,
			Body: fmt.Sprintf("tin %d", i)}
		if err := repo.Save(context.Background(), m); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	newest, err := repo.Thread(context.Background(), alice, bob, 0)
	if err != nil {
		t.Fatalf("newest page: %v", err)
	}
	if len(newest) != HistoryLimit {
		t.Fatalf("newest page has %d messages, want %d", len(newest), HistoryLimit)
	}
	// Oldest first, so the last one is the newest thing said.
	if newest[len(newest)-1].Body != fmt.Sprintf("tin %d", HistoryLimit+extra-1) {
		t.Fatalf("page is not oldest-first: ends with %q", newest[len(newest)-1].Body)
	}

	older, err := repo.Thread(context.Background(), alice, bob, newest[0].ID)
	if err != nil {
		t.Fatalf("older page: %v", err)
	}
	if len(older) != extra {
		t.Fatalf("older page has %d messages, want %d", len(older), extra)
	}
	if older[len(older)-1].ID >= newest[0].ID {
		t.Fatalf("pages overlap: older ends at %d, newer starts at %d",
			older[len(older)-1].ID, newest[0].ID)
	}

	// Past the beginning is empty, not the newest page again.
	none, err := repo.Thread(context.Background(), alice, bob, older[0].ID)
	if err != nil {
		t.Fatalf("past the start: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("reading past the oldest message returned %d rows", len(none))
	}
}
