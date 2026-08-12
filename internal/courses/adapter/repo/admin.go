package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/devforge/be/internal/courses/domain"
)

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

func isSlugTaken(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == uniqueViolation
}

// A row still pointed at from elsewhere: a played War Room scenario, which
// finished reports read back.
func isStillReferenced(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == foreignKeyViolation
}

// An insert naming a lab that is not there. Same class of mistake as a bad
// course id, and worth the same 404 rather than a 500.
func isMissingLab(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == foreignKeyViolation
}

// ByID is the admin-side read: unlike BySlug it does not filter on status,
// because editing a course is how a draft stops being one.
func (r *CourseRepo) ByID(ctx context.Context, id int64) (*domain.Course, error) {
	var row courseRow
	res := r.db.WithContext(ctx).Raw(
		`SELECT `+courseCols+` FROM courses c WHERE c.id = ?`, id,
	).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	out := row.toDomain()
	return &out, nil
}

// Create publishes in the same statement it inserts. published_at is derived
// from the status rather than sent by the client: it is the date shown on the
// course card, and a form should not be able to backdate it.
func (r *CourseRepo) Create(ctx context.Context, in domain.CourseInput) (*domain.Course, error) {
	var id int64
	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO courses (slug, title, description, image_url, level, status, published_at)
		 VALUES (?, ?, ?, ?, ?, ?, CASE WHEN ? = 'published' THEN now() END)
		 RETURNING id`,
		in.Slug, in.Title, in.Description, in.ImageURL, in.Level, in.Status, in.Status,
	).Scan(&id).Error
	if isSlugTaken(err) {
		return nil, domain.ErrSlugTaken
	}
	if err != nil {
		return nil, err
	}
	return r.ByID(ctx, id)
}

// Update keeps the first publication date. A course pulled back to draft and
// published again is the same course, and resetting the date every time would
// reorder the catalogue on an edit.
func (r *CourseRepo) Update(ctx context.Context, id int64, in domain.CourseInput) (*domain.Course, error) {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE courses SET slug = ?, title = ?, description = ?, image_url = ?,
		        level = ?, status = ?, updated_at = now(),
		        published_at = CASE
		            WHEN ? = 'published' AND published_at IS NULL THEN now()
		            ELSE published_at END
		  WHERE id = ?`,
		in.Slug, in.Title, in.Description, in.ImageURL, in.Level, in.Status, in.Status, id,
	)
	if isSlugTaken(res.Error) {
		return nil, domain.ErrSlugTaken
	}
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	return r.ByID(ctx, id)
}

// Delete takes the labs, tasks, enrolments and scores with it — every one of
// those tables cascades from courses. That is the point of the confirmation the
// admin screen asks for before calling this.
func (r *CourseRepo) Delete(ctx context.Context, id int64) error {
	res := r.db.WithContext(ctx).Exec(`DELETE FROM courses WHERE id = ?`, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// LevelExists is what keeps the foreign key from being the error message. The
// list is three rows and cached nowhere, so the query costs less than mapping a
// constraint violation back to a field name.
func (r *CourseRepo) LevelExists(ctx context.Context, slug string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(
		`SELECT count(*) FROM levels WHERE slug = ?`, slug,
	).Scan(&n).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return n > 0, err
}
