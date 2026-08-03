// Package chat is the shared room every signed-in student can talk in.
//
// One room, held in one process. The hub is a map in memory, so a second API
// instance would be a second room that cannot hear the first — see Hub for what
// that costs and what fixes it.
package chat

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

// MaxBody is the longest message accepted, in runes rather than bytes so a
// Vietnamese sentence is not cut shorter than an English one.
const MaxBody = 1000

// HistoryLimit is how much of the tail a joining client is handed. Enough to
// read the room, few enough to arrive in one frame.
const HistoryLimit = 50

type Message struct {
	ID     int64  `json:"id"`
	UserID *int64 `json:"user_id"`
	// As it was when the message was sent. A rename does not rewrite history.
	Username  string    `json:"username"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
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

// Save writes the message and fills in the id and timestamp the database
// assigned. Those are what make the ordering authoritative: two clients sending
// at once are ordered here, not by whichever frame arrived first.
func (r *Repo) Save(ctx context.Context, m *Message) error {
	return r.db.WithContext(ctx).Raw(
		`INSERT INTO chat_messages (user_id, username, body)
		 VALUES (?, ?, ?) RETURNING id, created_at`,
		m.UserID, m.Username, m.Body,
	).Row().Scan(&m.ID, &m.CreatedAt)
}

// Recent returns the tail in display order, oldest first. Read newest-first so
// the index is used, then reversed here rather than in SQL.
func (r *Repo) Recent(ctx context.Context) ([]Message, error) {
	out := []Message{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, username, body, created_at
		   FROM chat_messages ORDER BY id DESC LIMIT ?`, HistoryLimit,
	).Scan(&out).Error
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}
