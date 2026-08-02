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
	got := read(t, aliceWS)
	if got.Body != body || got.Username != aliceName {
		t.Fatalf("sender got %+v, want body %q from %q", got, body, aliceName)
	}
	if got.ID == 0 || got.CreatedAt.IsZero() {
		t.Fatalf("message has no id or timestamp: %+v — the database has to assign both", got)
	}

	onBob := read(t, bobWS)
	if onBob.ID != got.ID || onBob.Body != body {
		t.Fatalf("bob got %+v, want the same message alice sent", onBob)
	}

	// Persisted, so somebody joining later reads it.
	history, err := m.repo.Recent(context.Background())
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
	next := read(t, aliceWS)
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
	got := read(t, ws)
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
		var msg Message
		if err := ws.ReadJSON(&msg); err != nil {
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
	if err := ws.WriteJSON(incoming{Body: body}); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func read(t *testing.T, ws *websocket.Conn) Message {
	t.Helper()
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m Message
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return m
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
