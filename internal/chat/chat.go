// Package chat is the shared room, plus a direct thread between any two
// signed-in people.
//
// One room and any number of pairs, held in one process. The hub is a map in
// memory, so a second API instance would be a second set of rooms that cannot
// hear the first — see Hub for what that costs and what fixes it.
package chat

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

var (
	ErrNotFound = errors.New("message not found")
	// The caller is not the author. Answered the same way as "no such message"
	// at the edge, so probing ids says nothing about what exists.
	ErrNotAllowed = errors.New("not your message")
	// A direct message aimed at an account that is banned, deleted, or the
	// sender themselves.
	ErrNoPeer = errors.New("no such recipient")
)

// MaxBody is the longest message accepted, in runes rather than bytes so a
// Vietnamese sentence is not cut shorter than an English one.
const MaxBody = 1000

// HistoryLimit is how much of the tail a joining client is handed.
const HistoryLimit = 50

// PeopleLimit caps the directory search behind "start a conversation".
const PeopleLimit = 20

type Message struct {
	ID     int64  `json:"id"`
	UserID *int64 `json:"user_id"`
	// As it was when the message was sent. A rename does not rewrite history.
	Username string `json:"username"`
	// Nil in the shared room; the other side of the pair in a direct thread.
	PeerID    *int64    `json:"peer_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	// Set once the author changed it, so the screen can say so without
	// comparing timestamps that differ by a microsecond on every row.
	EditedAt *time.Time `json:"edited_at"`
	// Taken back. The row survives so ids and ordering do not reflow under
	// everybody's scroll position; Body is empty because the content is gone
	// rather than hidden by the client.
	DeletedAt *time.Time `json:"deleted_at"`
}

// Person is one entry in the directory used to start a conversation.
type Person struct {
	ID        int64   `json:"id"`
	Username  string  `json:"username"`
	AvatarURL *string `json:"avatar_url"`
}

// Conversation is one row of the sidebar: a thread with one other person,
// carrying enough of its last message to be recognised. The shared room is not
// in here — it always exists and needs no row to say so.
type Conversation struct {
	PeerID    int64   `json:"peer_id"`
	Username  string  `json:"username"`
	AvatarURL *string `json:"avatar_url"`
	LastBody  string  `json:"last_body"`
	LastAt    *string `json:"last_at"`
	// True when the last message was taken back, so the preview can say so
	// instead of showing an empty line.
	LastDeleted bool `json:"last_deleted"`
}

// Clean trims a submission and reports whether it is worth storing. Rejects the
// empty and the whitespace-only; truncates rather than refusing something too
// long, because losing the first thousand characters of a message to a hard
// error helps nobody.
func Clean(body string) (string, bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", false
	}
	if utf8.RuneCountInString(body) > MaxBody {
		body = string([]rune(body)[:MaxBody])
	}
	return body, true
}

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// A deleted message keeps its row and loses its text. Blanked here as well as
// on write so nothing downstream has to remember to.
const columns = `id, user_id, username, peer_id,
                 CASE WHEN deleted_at IS NULL THEN body ELSE '' END AS body,
                 created_at, edited_at, deleted_at`

// Save writes the message and fills in the id and timestamp the database
// assigned. Those are what make the ordering authoritative: two clients sending
// at once are ordered here, not by whichever frame arrived first.
func (r *Repo) Save(ctx context.Context, m *Message) error {
	return r.db.WithContext(ctx).Raw(
		`INSERT INTO chat_messages (user_id, username, peer_id, body)
		 VALUES (?, ?, ?, ?) RETURNING id, created_at`,
		m.UserID, m.Username, m.PeerID, m.Body,
	).Row().Scan(&m.ID, &m.CreatedAt)
}

// Room returns one page of the shared room in display order, oldest first. Read
// newest-first so the index is used, then reversed here rather than in SQL.
//
// before is the cursor for reading further back: the id of the oldest message
// already held, or 0 for the newest page. An id rather than a timestamp because
// ids are unique — two messages sharing a microsecond would make an offset-free
// cursor skip one of them.
func (r *Repo) Room(ctx context.Context, before int64) ([]Message, error) {
	return r.reversed(ctx,
		`SELECT `+columns+` FROM chat_messages
		  WHERE peer_id IS NULL AND (? = 0 OR id < ?)
		  ORDER BY id DESC LIMIT ?`, before, before, HistoryLimit)
}

// Thread returns one page of a direct conversation. Keyed by the pair sorted,
// so both directions of the same conversation are one thread. Same cursor as
// Room.
func (r *Repo) Thread(ctx context.Context, a, b, before int64) ([]Message, error) {
	lo, hi := Pair(a, b)
	return r.reversed(ctx,
		`SELECT `+columns+` FROM chat_messages
		  WHERE peer_id IS NOT NULL
		    AND LEAST(user_id, peer_id) = ? AND GREATEST(user_id, peer_id) = ?
		    AND (? = 0 OR id < ?)
		  ORDER BY id DESC LIMIT ?`, lo, hi, before, before, HistoryLimit)
}

// Pair puts two ids in the order the thread index expects. Exported because the
// hub keys its audiences the same way, and the two must not disagree.
func Pair(a, b int64) (int64, int64) {
	if a > b {
		return b, a
	}
	return a, b
}

func (r *Repo) reversed(ctx context.Context, q string, args ...any) ([]Message, error) {
	out := []Message{}
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// ByID reads one message, deleted or not. Used by the edit and delete paths,
// which need the author and the peer before deciding anything.
func (r *Repo) ByID(ctx context.Context, id int64) (*Message, error) {
	var m Message
	res := r.db.WithContext(ctx).Raw(
		`SELECT `+columns+` FROM chat_messages WHERE id = ?`, id,
	).Scan(&m)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &m, nil
}

// Edit rewrites a message the caller wrote. The author check is in the WHERE
// clause rather than in a read before it: check-then-write would let a row that
// changed in between be edited by somebody who is not its author.
//
// Refuses a message already taken back — editing it would put the content back
// on the screen of anyone who had not reloaded.
func (r *Repo) Edit(ctx context.Context, id, author int64, body string) (*Message, error) {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE chat_messages SET body = ?, edited_at = now()
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		body, id, author,
	)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotAllowed
	}
	return r.ByID(ctx, id)
}

// Delete takes a message back. Same shape as Edit, and idempotent: deleting one
// that is already gone is the state the caller asked for, not an error.
func (r *Repo) Delete(ctx context.Context, id, author int64) (*Message, error) {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE chat_messages SET body = '', deleted_at = now()
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, id, author)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		m, err := r.ByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if m.UserID == nil || *m.UserID != author {
			return nil, ErrNotAllowed
		}
		return m, nil
	}
	return r.ByID(ctx, id)
}

// Conversations lists every direct thread the caller is part of, most recent
// first. One query over the messages themselves rather than a conversations
// table that has to be kept in step with them.
func (r *Repo) Conversations(ctx context.Context, me int64) ([]Conversation, error) {
	out := []Conversation{}
	err := r.db.WithContext(ctx).Raw(
		`WITH mine AS (
		   SELECT CASE WHEN user_id = ? THEN peer_id ELSE user_id END AS other,
		          id, body, created_at, deleted_at
		     FROM chat_messages
		    WHERE peer_id IS NOT NULL AND (user_id = ? OR peer_id = ?)
		 ),
		 last AS (
		   SELECT DISTINCT ON (other) other, body, created_at, deleted_at
		     FROM mine ORDER BY other, id DESC
		 )
		 SELECT l.other AS peer_id, u.username, u.avatar_url,
		        CASE WHEN l.deleted_at IS NULL THEN l.body ELSE '' END AS last_body,
		        to_char(l.created_at AT TIME ZONE 'UTC',
		                'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS last_at,
		        l.deleted_at IS NOT NULL AS last_deleted
		   FROM last l JOIN users u ON u.id = l.other
		  ORDER BY l.created_at DESC`,
		me, me, me,
	).Scan(&out).Error
	return out, err
}

// People is the directory behind "message someone". Active accounts only, and
// never the caller: a thread with yourself is the one pair the schema refuses.
func (r *Repo) People(ctx context.Context, me int64, q string) ([]Person, error) {
	out := []Person{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, username, avatar_url FROM users
		  WHERE id <> ? AND status = 'active' AND username ILIKE ?
		  ORDER BY username LIMIT ?`,
		me, "%"+strings.TrimSpace(q)+"%", PeopleLimit,
	).Scan(&out).Error
	return out, err
}

// CanReceive reports whether an account may be written to. Checked before a
// direct message is stored, so a peer id typed by hand cannot open a thread with
// a banned or deleted account.
func (r *Repo) CanReceive(ctx context.Context, id int64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(
		`SELECT count(*) FROM users WHERE id = ? AND status = 'active'`, id,
	).Scan(&n).Error
	return n > 0, err
}
