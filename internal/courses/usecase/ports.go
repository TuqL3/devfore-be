package usecase

import (
	"context"

	"github.com/devforge/be/internal/courses/domain"
)

type CourseRepository interface {
	ListPublished(ctx context.Context, f domain.CourseFilter) ([]domain.Course, error)
	ListLevels(ctx context.Context) ([]domain.Level, error)
	BySlug(ctx context.Context, slug string) (*domain.Course, error)
	LabsByCourse(ctx context.Context, courseID int64) ([]domain.Lab, error)
	Reviews(ctx context.Context, courseID int64) ([]domain.Review, error)
	Enroll(ctx context.Context, userID, courseID int64) error
	Unenroll(ctx context.Context, userID, courseID int64) error
	IsEnrolled(ctx context.Context, userID, courseID int64) (bool, error)
	Leaderboard(ctx context.Context, courseID int64) ([]domain.LeaderRow, error)
}
