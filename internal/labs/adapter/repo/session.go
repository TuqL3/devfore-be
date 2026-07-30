package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/devforge/be/internal/labs/domain"
)

// uniqueViolation is what Postgres answers when the one-running-session-per-user
// index rejects a second insert.
const uniqueViolation = "23505"

type SessionRepo struct{ db *gorm.DB }

func NewSessionRepo(db *gorm.DB) *SessionRepo { return &SessionRepo{db: db} }

// SpecBySlug reads the lab together with the image it is pinned to. The join is
// inner on purpose: a lab with no lab_images row has nothing to start, and
// guessing a default would mean running students on an image nobody chose.
func (r *SessionRepo) SpecBySlug(ctx context.Context, labSlug string) (*domain.Spec, error) {
	var row struct {
		LabID    int64
		LabSlug  string
		LabTitle string
		Image    string
	}
	res := r.db.WithContext(ctx).Raw(
		`SELECT l.id AS lab_id, l.slug AS lab_slug, l.title AS lab_title,
		        i.name || ':' || i.tag AS image
		   FROM labs l
		   JOIN lab_images i ON i.id = l.lab_image_id
		   JOIN courses c    ON c.id = l.course_id
		  WHERE l.slug = ? AND i.active AND c.status = 'published'`, labSlug,
	).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrLabNotFound
	}
	return &domain.Spec{LabID: row.LabID, LabSlug: row.LabSlug, LabTitle: row.LabTitle, Image: row.Image}, nil
}

// Create claims the user's one running slot. The unique index is what actually
// decides: two requests racing each other both reach here, and the loser is told
// a session is already running rather than being handed a second container.
func (r *SessionRepo) Create(ctx context.Context, s *domain.Session) error {
	err := r.db.WithContext(ctx).Exec(
		`INSERT INTO lab_sessions (id, user_id, lab_id, expires_at)
		 VALUES (?, ?, ?, ?)`, s.ID, s.UserID, s.LabID, s.ExpiresAt,
	).Error
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == uniqueViolation {
		return domain.ErrAlreadyRunning
	}
	return err
}

func (r *SessionRepo) SetContainer(ctx context.Context, id, containerID string) error {
	return r.db.WithContext(ctx).Exec(
		`UPDATE lab_sessions SET container_id = ? WHERE id = ?`, containerID, id,
	).Error
}

func (r *SessionRepo) ByID(ctx context.Context, id string) (*domain.Session, error) {
	var s domain.Session
	res := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, lab_id, container_id, status, started_at, expires_at, ended_at
		   FROM lab_sessions WHERE id = ?`, id,
	).Scan(&s)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	return &s, nil
}

func (r *SessionRepo) RunningByUser(ctx context.Context, userID int64) (*domain.Session, error) {
	var s domain.Session
	res := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, lab_id, container_id, status, started_at, expires_at, ended_at
		   FROM lab_sessions WHERE user_id = ? AND status = 'running'`, userID,
	).Scan(&s)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	return &s, nil
}

// End is written so that only a running row moves. Two reapers, or a reaper
// racing a student pressing Stop, then agree on one outcome instead of both
// counting the same session.
func (r *SessionRepo) End(ctx context.Context, id string, status domain.Status) (bool, error) {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE lab_sessions SET status = ?, ended_at = now()
		  WHERE id = ? AND status = 'running'`, string(status), id,
	)
	return res.RowsAffected > 0, res.Error
}

// DueForReaping returns sessions whose deadline has passed. Reading the deadline
// from the table rather than from a timer is what makes a restarted server still
// clean up after itself.
func (r *SessionRepo) DueForReaping(ctx context.Context, limit int) ([]domain.Session, error) {
	sessions := []domain.Session{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, user_id, lab_id, container_id, status, started_at, expires_at, ended_at
		   FROM lab_sessions
		  WHERE status = 'running' AND expires_at <= ?
		  ORDER BY expires_at
		  LIMIT ?`, time.Now(), limit,
	).Scan(&sessions).Error
	return sessions, err
}
