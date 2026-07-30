package rest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase"
)

// A websocket handshake carries the user's cookies exactly like any other
// request, so without an origin check any page on the internet could open one
// against a logged-in student and drive their shell. The browser cannot be
// asked to enforce it — same-origin policy does not apply to websockets — so
// the allowlist is checked here.
type Terminal struct {
	uc       *usecase.Labs
	upgrader websocket.Upgrader
}

func NewTerminal(uc *usecase.Labs, allowedOrigins []string) *Terminal {
	return &Terminal{
		uc: uc,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				// A missing Origin is not a browser, so it is not the attack this
				// guards against; the session cookie still has to be valid.
				return origin == "" || slices.Contains(allowedOrigins, origin)
			},
		},
	}
}

const (
	// The browser has to prove it is still there. A student who suspends their
	// laptop should not hold a container open until the reaper notices.
	pongWait   = 60 * time.Second
	pingEvery  = 25 * time.Second
	writeWait  = 10 * time.Second
	readLimit  = 32 << 10
	streamCopy = 32 << 10
)

type controlMessage struct {
	Type string `json:"type"`
	Rows uint   `json:"rows"`
	Cols uint   `json:"cols"`
}

func (t *Terminal) Handle(c *gin.Context) {
	// Everything that can fail with a readable status is checked before the
	// upgrade: once the connection is a websocket, an error is a close frame the
	// client has to decode rather than a status it already understands.
	s, err := t.uc.Live(c.Request.Context(), c.Param("id"), userID(c))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
		return
	case errors.Is(err, domain.ErrNotRunning):
		abort(c, http.StatusGone, "phiên lab đã kết thúc")
		return
	case err != nil:
		serverError(c, err)
		return
	}

	ws, err := t.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Upgrade has already written its own response by this point.
		slog.Warn("terminal upgrade", "session", s.ID, "err", err)
		return
	}
	defer ws.Close()

	// The stream outlives the request context, which gin cancels as soon as the
	// handler returns, so the shell gets its own lifetime bounded by the session.
	ctx, cancel := context.WithDeadline(context.WithoutCancel(c.Request.Context()), s.ExpiresAt)
	defer cancel()

	stream, execID, err := t.uc.Attach(ctx, s)
	if err != nil {
		slog.Error("terminal attach", "session", s.ID, "err", err)
		_ = ws.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "không mở được terminal"),
			time.Now().Add(writeWait))
		return
	}
	defer stream.Close()

	slog.Info("terminal open", "session", s.ID, "user", s.UserID)
	t.pump(ctx, cancel, ws, stream, execID, s)
	slog.Info("terminal closed", "session", s.ID, "user", s.UserID)
}

// pump wires the two directions together and returns once either gives out.
// Whichever side stops first cancels the context, which tears down the other.
func (t *Terminal) pump(
	ctx context.Context, cancel context.CancelFunc,
	ws *websocket.Conn, stream io.ReadWriteCloser, execID string, s *domain.Session,
) {
	go t.containerToClient(ctx, cancel, ws, stream)
	go t.keepAlive(ctx, ws)
	t.clientToContainer(ctx, cancel, ws, stream, execID, s)
}

// clientToContainer runs on the handler goroutine: it owns the websocket reader,
// which gorilla allows only one of.
func (t *Terminal) clientToContainer(
	ctx context.Context, cancel context.CancelFunc,
	ws *websocket.Conn, stream io.Writer, execID string, s *domain.Session,
) {
	defer cancel()
	ws.SetReadLimit(readLimit)
	_ = ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		kind, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		// Text frames are control, binary frames are keystrokes. Splitting them by
		// frame type means a student typing JSON at the shell cannot resize their
		// own window, or anything else.
		if kind == websocket.TextMessage {
			var msg controlMessage
			if err := json.Unmarshal(data, &msg); err != nil || msg.Type != "resize" {
				continue
			}
			if msg.Rows == 0 || msg.Cols == 0 || msg.Rows > 500 || msg.Cols > 500 {
				continue
			}
			if err := t.uc.Resize(ctx, execID, msg.Rows, msg.Cols); err != nil {
				slog.Warn("terminal resize", "session", s.ID, "err", err)
			}
			continue
		}
		if _, err := stream.Write(data); err != nil {
			return
		}
	}
}

func (t *Terminal) containerToClient(
	ctx context.Context, cancel context.CancelFunc,
	ws *websocket.Conn, stream io.Reader,
) {
	defer cancel()
	buf := make([]byte, streamCopy)
	for {
		n, err := stream.Read(buf)
		if n > 0 {
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
		if err != nil {
			// The shell exited or the session hit its deadline. Say which, so the
			// client can tell "you typed exit" from "your hour is up".
			msg := "shell đã thoát"
			if ctx.Err() != nil {
				msg = "phiên lab đã hết giờ"
			}
			_ = ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, msg),
				time.Now().Add(writeWait))
			return
		}
	}
}

// keepAlive is also what enforces the deadline: when the context expires the
// close frame goes out here rather than waiting for the student to type.
func (t *Terminal) keepAlive(ctx context.Context, ws *websocket.Conn) {
	tick := time.NewTicker(pingEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "phiên lab đã hết giờ"),
				time.Now().Add(writeWait))
			_ = ws.Close()
			return
		case <-tick.C:
			_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				return
			}
		}
	}
}
