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

// SpecBySlug reads the lab together with whichever runtime it is pinned to: an
// image for a container lab, a scenario for a sim lab.
//
// The image join is left rather than inner because a sim lab has no lab_images
// row at all — and `i.active` has to sit in the join condition, not in the WHERE
// clause, or a null image fails the test and the left join collapses back into
// an inner one. What the inner join used to enforce is enforced below instead: a
// lab with neither runtime is still not found, because starting it would mean
// guessing an image nobody chose.
func (r *SessionRepo) SpecBySlug(ctx context.Context, labSlug string) (*domain.Spec, error) {
	var row struct {
		LabID       int64
		LabSlug     string
		LabTitle    string
		CourseSlug  string
		Image       string
		SimScenario []byte
	}
	res := r.db.WithContext(ctx).Raw(
		`SELECT l.id AS lab_id, l.slug AS lab_slug, l.title AS lab_title,
		        c.slug AS course_slug,
		        COALESCE(i.name || ':' || i.tag, '') AS image,
		        l.sim_scenario
		   FROM labs l
		   LEFT JOIN lab_images i ON i.id = l.lab_image_id AND i.active
		   JOIN courses c         ON c.id = l.course_id
		  WHERE l.slug = ? AND c.status = 'published'`, labSlug,
	).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrLabNotFound
	}
	if row.Image == "" && len(row.SimScenario) == 0 {
		// No image, or an image that was deactivated, and no scenario either.
		// Same answer the inner join gave before: there is nothing to start.
		return nil, domain.ErrLabNotFound
	}
	return &domain.Spec{
		LabID:       row.LabID,
		LabSlug:     row.LabSlug,
		LabTitle:    row.LabTitle,
		CourseSlug:  row.CourseSlug,
		Image:       row.Image,
		SimScenario: row.SimScenario,
	}, nil
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

// sessionSelect carries the lab and course slug alongside the row so a client can
// navigate back to a session it already owns. Both joins are left joins and
// neither filters on course status: a session that outlived its course being
// unpublished still has a container attached to it, and the student still has to
// be able to reach that session to end it.
const sessionSelect = `SELECT s.id, s.user_id, s.lab_id, s.container_id, s.status,
       s.started_at, s.expires_at, s.ended_at,
       COALESCE(l.slug, '') AS lab_slug, COALESCE(c.slug, '') AS course_slug
  FROM lab_sessions s
  LEFT JOIN labs l    ON l.id = s.lab_id
  LEFT JOIN courses c ON c.id = l.course_id`

func (r *SessionRepo) ByID(ctx context.Context, id string) (*domain.Session, error) {
	var s domain.Session
	res := r.db.WithContext(ctx).Raw(sessionSelect+` WHERE s.id = ?`, id).Scan(&s)
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
		sessionSelect+` WHERE s.user_id = ? AND s.status = 'running'`, userID,
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

// IsEnrolled reports whether the student signed up for the course this lab
// belongs to. Answered in one query from the lab's slug: the caller has the slug
// and nothing else at the point the question needs asking, and looking the
// course up separately would be a second round trip to learn the same thing.
func (r *SessionRepo) IsEnrolled(ctx context.Context, userID int64, labSlug string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(
		`SELECT count(*) FROM enrollments e
		   JOIN labs l ON l.course_id = e.course_id
		  WHERE e.user_id = ? AND l.slug = ?`, userID, labSlug,
	).Scan(&n).Error
	return n > 0, err
}
