package usecase

import (
	"context"
	"testing"

	"github.com/devforge/be/internal/courses/domain"
)

type fakeRepo struct {
	course   *domain.Course
	labs     []domain.Lab
	enrolled bool
}

func (f *fakeRepo) ListPublished(context.Context) ([]domain.Course, error) { return nil, nil }
func (f *fakeRepo) BySlug(_ context.Context, slug string) (*domain.Course, error) {
	if f.course == nil || f.course.Slug != slug {
		return nil, domain.ErrNotFound
	}
	c := *f.course
	return &c, nil
}
func (f *fakeRepo) LabsByCourse(context.Context, int64) ([]domain.Lab, error) { return f.labs, nil }
func (f *fakeRepo) Reviews(context.Context, int64) ([]domain.Review, error)   { return nil, nil }
func (f *fakeRepo) Enroll(context.Context, int64, int64) error                { return nil }
func (f *fakeRepo) IsEnrolled(context.Context, int64, int64) (bool, error)    { return f.enrolled, nil }
func (f *fakeRepo) Leaderboard(context.Context, int64) ([]domain.LeaderRow, error) {
	return nil, nil
}

func TestDetail(t *testing.T) {
	repo := &fakeRepo{
		course:   &domain.Course{ID: 1, Slug: "linux", Status: "published"},
		labs:     []domain.Lab{{ID: 1}, {ID: 2}},
		enrolled: true,
	}
	uc := NewCourses(repo)

	c, err := uc.Detail(context.Background(), "linux", 0)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if c.LabCount != 2 || len(c.Labs) != 2 {
		t.Fatalf("want 2 labs, got %d", c.LabCount)
	}
	if c.Enrolled {
		t.Fatal("anonymous must not be enrolled")
	}

	c, _ = uc.Detail(context.Background(), "linux", 42)
	if !c.Enrolled {
		t.Fatal("want enrolled=true for user")
	}

	if _, err := uc.Detail(context.Background(), "nope", 0); err != domain.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
