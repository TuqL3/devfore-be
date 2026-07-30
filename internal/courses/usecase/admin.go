package usecase

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/devforge/be/internal/courses/domain"
)

// A slug is the course's URL. Lowercase words joined by single hyphens is the
// whole rule, checked here rather than left to the database, so a bad one comes
// back as a field error instead of a 500.
var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

const (
	maxSlug        = 100
	maxTitle       = 200
	maxDescription = 2000
)

func (c *Courses) ListAll(ctx context.Context, f domain.CourseFilter) ([]domain.Course, error) {
	f.Query = strings.TrimSpace(f.Query)
	f.IncludeDrafts = true
	return c.repo.ListPublished(ctx, f)
}

func (c *Courses) Create(ctx context.Context, in domain.CourseInput) (*domain.Course, error) {
	in, err := c.clean(ctx, in)
	if err != nil {
		return nil, err
	}
	return c.repo.Create(ctx, in)
}

func (c *Courses) Update(ctx context.Context, id int64, in domain.CourseInput) (*domain.Course, error) {
	in, err := c.clean(ctx, in)
	if err != nil {
		return nil, err
	}
	return c.repo.Update(ctx, id, in)
}

func (c *Courses) Delete(ctx context.Context, id int64) error {
	return c.repo.Delete(ctx, id)
}

// clean validates and normalises in one pass, so create and update cannot drift
// apart on what they accept.
func (c *Courses) clean(ctx context.Context, in domain.CourseInput) (domain.CourseInput, error) {
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Level = strings.TrimSpace(in.Level)
	in.Status = strings.TrimSpace(in.Status)

	switch {
	case in.Slug == "":
		return in, domain.InvalidInput{Field: "slug", Message: "slug không được để trống"}
	case len(in.Slug) > maxSlug:
		return in, domain.InvalidInput{Field: "slug", Message: "slug quá dài"}
	case !slugRe.MatchString(in.Slug):
		return in, domain.InvalidInput{
			Field:   "slug",
			Message: "slug chỉ gồm chữ thường, số và dấu gạch ngang",
		}
	case in.Title == "":
		return in, domain.InvalidInput{Field: "title", Message: "tiêu đề không được để trống"}
	case len(in.Title) > maxTitle:
		return in, domain.InvalidInput{Field: "title", Message: "tiêu đề quá dài"}
	case len(in.Description) > maxDescription:
		return in, domain.InvalidInput{Field: "description", Message: "mô tả quá dài"}
	case in.Status != "draft" && in.Status != "published":
		return in, domain.InvalidInput{Field: "status", Message: "trạng thái không hợp lệ"}
	}

	// An empty image field is no image, not an empty string: the column is
	// nullable and the card checks for null to fall back to a placeholder.
	if in.ImageURL != nil {
		trimmed := strings.TrimSpace(*in.ImageURL)
		if trimmed == "" {
			in.ImageURL = nil
		} else {
			u, err := url.Parse(trimmed)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return in, domain.InvalidInput{
					Field:   "image_url",
					Message: "ảnh phải là đường dẫn http(s)",
				}
			}
			in.ImageURL = &trimmed
		}
	}

	ok, err := c.repo.LevelExists(ctx, in.Level)
	if err != nil {
		return in, err
	}
	if !ok {
		return in, domain.InvalidInput{Field: "level", Message: "cấp độ không hợp lệ"}
	}
	return in, nil
}
