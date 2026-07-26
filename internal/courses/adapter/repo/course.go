package repo

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/devforge/be/internal/courses/domain"
	"github.com/devforge/be/internal/courses/usecase"
)

var _ usecase.CourseRepository = (*CourseRepo)(nil)

type CourseRepo struct{ db *gorm.DB }

func NewCourseRepo(db *gorm.DB) *CourseRepo { return &CourseRepo{db: db} }

type courseRow struct {
	ID           int64
	Slug         string
	Title        string
	Description  string
	ImageURL     *string
	Level        string
	Status       string
	PublishedAt  *time.Time
	UpdatedAt    time.Time
	LabCount     int
	StudentCount int64
}

func (r courseRow) toDomain() domain.Course {
	return domain.Course{
		ID:           r.ID,
		Slug:         r.Slug,
		Title:        r.Title,
		Description:  r.Description,
		ImageURL:     r.ImageURL,
		Level:        r.Level,
		Status:       r.Status,
		PublishedAt:  r.PublishedAt,
		UpdatedAt:    r.UpdatedAt,
		LabCount:     r.LabCount,
		StudentCount: r.StudentCount,
	}
}

const courseCols = `c.id, c.slug, c.title, c.description, c.image_url, c.level, c.status,
	c.published_at, c.updated_at,
	(SELECT count(*) FROM labs l WHERE l.course_id = c.id)        AS lab_count,
	(SELECT count(*) FROM enrollments e WHERE e.course_id = c.id) AS student_count`

func (r *CourseRepo) ListPublished(ctx context.Context) ([]domain.Course, error) {
	var rows []courseRow
	err := r.db.WithContext(ctx).Raw(
		`SELECT ` + courseCols + ` FROM courses c
		 WHERE c.status = 'published'
		 ORDER BY c.published_at DESC NULLS LAST, c.id DESC`,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Course, len(rows))
	for i, row := range rows {
		out[i] = row.toDomain()
	}
	return out, nil
}

func (r *CourseRepo) BySlug(ctx context.Context, slug string) (*domain.Course, error) {
	var row courseRow
	res := r.db.WithContext(ctx).Raw(
		`SELECT `+courseCols+` FROM courses c
		 WHERE c.slug = ? AND c.status = 'published'`, slug,
	).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	course := row.toDomain()
	return &course, nil
}

func (r *CourseRepo) LabsByCourse(ctx context.Context, courseID int64) ([]domain.Lab, error) {
	var labs []domain.Lab
	err := r.db.WithContext(ctx).Raw(
		`SELECT l.id, l.slug, l.title, l.description_md, l.duration_minutes, l.order_idx,
			(SELECT count(*) FROM lab_tasks t WHERE t.lab_id = l.id)               AS task_count,
			COALESCE((SELECT sum(points) FROM lab_tasks t WHERE t.lab_id = l.id), 0) AS points
		 FROM labs l WHERE l.course_id = ? ORDER BY l.order_idx, l.id`, courseID,
	).Scan(&labs).Error
	return labs, err
}

func (r *CourseRepo) Reviews(ctx context.Context, courseID int64) ([]domain.Review, error) {
	var reviews []domain.Review
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, title, content_md, order_idx FROM reviews
		 WHERE course_id = ? ORDER BY order_idx, id`, courseID,
	).Scan(&reviews).Error
	return reviews, err
}

func (r *CourseRepo) Enroll(ctx context.Context, userID, courseID int64) error {
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO enrollments (user_id, course_id) VALUES (?, ?)
		 ON CONFLICT DO NOTHING`, userID, courseID,
	).Error
}

func (r *CourseRepo) IsEnrolled(ctx context.Context, userID, courseID int64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("enrollments").
		Where("user_id = ? AND course_id = ?", userID, courseID).Count(&n).Error
	return n > 0, err
}

func (r *CourseRepo) Leaderboard(ctx context.Context, courseID int64) ([]domain.LeaderRow, error) {
	return []domain.LeaderRow{}, nil
}
