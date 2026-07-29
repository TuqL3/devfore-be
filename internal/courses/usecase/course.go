package usecase

import (
	"context"
	"strings"

	"github.com/devforge/be/internal/courses/adapter/repo"
	"github.com/devforge/be/internal/courses/domain"
)

type Courses struct {
	repo *repo.CourseRepo
}

func NewCourses(repo *repo.CourseRepo) *Courses {
	return &Courses{repo: repo}
}

func (c *Courses) List(ctx context.Context, f domain.CourseFilter) ([]domain.Course, error) {
	f.Query = strings.TrimSpace(f.Query)
	return c.repo.ListPublished(ctx, f)
}

func (c *Courses) Levels(ctx context.Context) ([]domain.Level, error) {
	return c.repo.ListLevels(ctx)
}

func (c *Courses) Detail(ctx context.Context, slug string, userID int64) (*domain.Course, error) {
	course, err := c.repo.BySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	labs, err := c.repo.LabsByCourse(ctx, course.ID)
	if err != nil {
		return nil, err
	}
	course.Labs = labs
	course.LabCount = len(labs)
	if userID > 0 {
		course.Enrolled, err = c.repo.IsEnrolled(ctx, userID, course.ID)
		if err != nil {
			return nil, err
		}
	}
	return course, nil
}

// Lab resolves the course first so an unpublished course hides its labs: BySlug
// only returns published rows, and everything here hangs off its id.
func (c *Courses) Lab(ctx context.Context, courseSlug, labSlug string) (*domain.Lab, error) {
	course, err := c.repo.BySlug(ctx, courseSlug)
	if err != nil {
		return nil, err
	}
	lab, err := c.repo.LabBySlug(ctx, course.ID, labSlug)
	if err != nil {
		return nil, err
	}
	lab.Tasks, err = c.repo.TasksByLab(ctx, lab.ID)
	if err != nil {
		return nil, err
	}
	return lab, nil
}

func (c *Courses) Enroll(ctx context.Context, userID int64, slug string) error {
	course, err := c.repo.BySlug(ctx, slug)
	if err != nil {
		return err
	}
	return c.repo.Enroll(ctx, userID, course.ID)
}

func (c *Courses) Unenroll(ctx context.Context, userID int64, slug string) error {
	course, err := c.repo.BySlug(ctx, slug)
	if err != nil {
		return err
	}
	return c.repo.Unenroll(ctx, userID, course.ID)
}

func (c *Courses) Reviews(ctx context.Context, slug string) ([]domain.Review, error) {
	course, err := c.repo.BySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	return c.repo.Reviews(ctx, course.ID)
}

func (c *Courses) Leaderboard(ctx context.Context, slug string) ([]domain.LeaderRow, error) {
	course, err := c.repo.BySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	return c.repo.Leaderboard(ctx, course.ID)
}
