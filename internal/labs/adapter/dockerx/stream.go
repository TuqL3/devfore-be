package dockerx

import (
	"io"
	"net"
	"strings"
)

// hijacked joins the two halves docker hands back from an attach — a net.Conn to
// write to and a buffered reader to read from — into something the websocket
// pump can treat as one connection.
type hijacked struct {
	conn net.Conn
	r    io.Reader
}

func (h *hijacked) Read(p []byte) (int, error)  { return h.r.Read(p) }
func (h *hijacked) Write(p []byte) (int, error) { return h.conn.Write(p) }

func (h *hijacked) Close() error {
	// CloseWrite sends EOF to the shell so it exits on its own; the full Close
	// then tears down the socket whether or not it took the hint.
	if cw, ok := h.conn.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
	return h.conn.Close()
}

// checkOutputLimit caps what a check script can hand back. A task whose script
// loops printing forever should fail on its exit code, not by filling the
// response with a megabyte of noise.
const checkOutputLimit = 8 << 10

type limitedBuffer struct {
	b        strings.Builder
	overflow bool
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	if room := checkOutputLimit - w.b.Len(); room > 0 {
		if len(p) > room {
			w.b.Write(p[:room])
			w.overflow = true
		} else {
			w.b.Write(p)
		}
	} else if len(p) > 0 {
		w.overflow = true
	}
	// Always report the full length: a short write makes stdcopy stop early and
	// report an error, and a talkative script is not an error.
	return len(p), nil
}

func (w *limitedBuffer) String() string {
	if w.overflow {
		return w.b.String() + "\n… (đã cắt bớt)"
	}
	return w.b.String()
}
