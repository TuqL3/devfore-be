package usecase

import (
	"context"
	"strings"

	"github.com/devforge/be/internal/courses/domain"
)

const (
	maxDescriptionMD = 20000
	maxCheckScript   = 4000
	maxHint          = 2000
	maxDuration      = 600 // ten hours; past this it is a typo, not a lab
	maxPoints        = 1000
	maxOptions       = 10
	maxOptionText    = 500
	maxCommands      = 10
	maxCommandLen    = 200
)

func (c *Courses) AdminLabs(ctx context.Context, courseID int64) ([]domain.Lab, error) {
	// Reading the course first is what turns a bad id into a 404 rather than an
	// empty list that looks like a course with no labs.
	if _, err := c.repo.ByID(ctx, courseID); err != nil {
		return nil, err
	}
	return c.repo.AdminLabs(ctx, courseID)
}

func (c *Courses) CreateLab(ctx context.Context, courseID int64, in domain.LabInput) (*domain.Lab, error) {
	if _, err := c.repo.ByID(ctx, courseID); err != nil {
		return nil, err
	}
	in, err := c.cleanLab(ctx, in)
	if err != nil {
		return nil, err
	}
	return c.repo.CreateLab(ctx, courseID, in)
}

func (c *Courses) UpdateLab(ctx context.Context, labID int64, in domain.LabInput) (*domain.Lab, error) {
	in, err := c.cleanLab(ctx, in)
	if err != nil {
		return nil, err
	}
	return c.repo.UpdateLab(ctx, labID, in)
}

func (c *Courses) DeleteLab(ctx context.Context, labID int64) error {
	return c.repo.DeleteLab(ctx, labID)
}

func (c *Courses) AdminTasks(ctx context.Context, labID int64) ([]domain.AdminTask, error) {
	if _, err := c.repo.AdminLab(ctx, labID); err != nil {
		return nil, err
	}
	return c.repo.AdminTasks(ctx, labID)
}

func (c *Courses) CreateTask(ctx context.Context, labID int64, in domain.TaskInput) (*domain.AdminTask, error) {
	if _, err := c.repo.AdminLab(ctx, labID); err != nil {
		return nil, err
	}
	in, err := cleanTask(in)
	if err != nil {
		return nil, err
	}
	return c.repo.CreateTask(ctx, labID, in)
}

func (c *Courses) UpdateTask(ctx context.Context, taskID int64, in domain.TaskInput) (*domain.AdminTask, error) {
	in, err := cleanTask(in)
	if err != nil {
		return nil, err
	}
	return c.repo.UpdateTask(ctx, taskID, in)
}

func (c *Courses) DeleteTask(ctx context.Context, taskID int64) error {
	return c.repo.DeleteTask(ctx, taskID)
}

func (c *Courses) LabImages(ctx context.Context) ([]domain.LabImage, error) {
	return c.repo.LabImages(ctx)
}

func (c *Courses) cleanLab(ctx context.Context, in domain.LabInput) (domain.LabInput, error) {
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Title = strings.TrimSpace(in.Title)
	in.DescriptionMD = strings.TrimSpace(in.DescriptionMD)

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
	case len(in.DescriptionMD) > maxDescriptionMD:
		return in, domain.InvalidInput{Field: "description_md", Message: "hướng dẫn quá dài"}
	case in.DurationMinutes < 1 || in.DurationMinutes > maxDuration:
		return in, domain.InvalidInput{
			Field:   "duration_minutes",
			Message: "thời lượng phải từ 1 đến 600 phút",
		}
	case in.OrderIdx < 0:
		return in, domain.InvalidInput{Field: "order_idx", Message: "thứ tự không được âm"}
	}

	// A lab with no image cannot be started, but saving one is how a draft gets
	// written before the image exists. A wrong id is the error worth refusing.
	if in.LabImageID != nil {
		ok, err := c.repo.LabImageExists(ctx, *in.LabImageID)
		if err != nil {
			return in, err
		}
		if !ok {
			return in, domain.InvalidInput{Field: "lab_image_id", Message: "image không tồn tại"}
		}
	}
	return in, nil
}

// cleanTask needs no repository: everything it checks is in the input. The check
// script itself is deliberately not validated beyond a length — it is shell, and
// guessing at what is valid shell here would reject things that work.
func cleanTask(in domain.TaskInput) (domain.TaskInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Hint = strings.TrimSpace(in.Hint)
	in.CheckScript = strings.TrimSpace(in.CheckScript)
	if in.Kind == "" {
		in.Kind = domain.KindScript
	}

	switch {
	case in.Title == "":
		return in, domain.InvalidInput{Field: "title", Message: "đề bài không được để trống"}
	case len(in.Title) > maxTitle:
		return in, domain.InvalidInput{Field: "title", Message: "đề bài quá dài"}
	case len(in.Hint) > maxHint:
		return in, domain.InvalidInput{Field: "hint", Message: "gợi ý quá dài"}
	case len(in.CheckScript) > maxCheckScript:
		return in, domain.InvalidInput{Field: "check_script", Message: "script quá dài"}
	case in.Points < 0 || in.Points > maxPoints:
		return in, domain.InvalidInput{Field: "points", Message: "điểm phải từ 0 đến 1000"}
	case in.OrderIdx < 0:
		return in, domain.InvalidInput{Field: "order_idx", Message: "thứ tự không được âm"}
	case in.Kind != domain.KindScript && in.Kind != domain.KindChoice &&
		in.Kind != domain.KindCommand:
		return in, domain.InvalidInput{Field: "kind", Message: "loại nhiệm vụ không hợp lệ"}
	}

	if in.Kind == domain.KindScript {
		// Leftovers from a task that used to be another kind would sit in the row
		// unused and reappear if it ever switched back.
		in.Options = nil
		in.ExpectedCommands = ""
		return in, nil
	}

	if in.Kind == domain.KindCommand {
		in.Options = nil
		in.CheckScript = ""

		commands := []string{}
		for _, line := range strings.Split(in.ExpectedCommands, "\n") {
			// Collapsed here so the same normalisation the grader applies to the
			// history is already baked into what is stored.
			if line = strings.Join(strings.Fields(line), " "); line != "" {
				commands = append(commands, line)
			}
		}
		switch {
		case len(commands) == 0:
			return in, domain.InvalidInput{
				Field:   "expected_commands",
				Message: "cần ít nhất 1 câu lệnh được chấp nhận",
			}
		case len(commands) > maxCommands:
			return in, domain.InvalidInput{Field: "expected_commands", Message: "quá nhiều câu lệnh"}
		}
		for _, cmd := range commands {
			if len(cmd) > maxCommandLen {
				return in, domain.InvalidInput{
					Field:   "expected_commands",
					Message: "một câu lệnh quá dài",
				}
			}
		}
		in.ExpectedCommands = strings.Join(commands, "\n")
		return in, nil
	}

	// From here the task is a choice question: graded by comparing ticks, so the
	// script is what becomes meaningless and the options carry every rule.
	in.CheckScript = ""
	in.ExpectedCommands = ""
	options := make([]domain.Option, 0, len(in.Options))
	correct := 0
	for _, o := range in.Options {
		o.Text = strings.TrimSpace(o.Text)
		if o.Text == "" {
			// A blank option is a row the author started and abandoned, not an
			// answer worth showing.
			continue
		}
		if len(o.Text) > maxOptionText {
			return in, domain.InvalidInput{Field: "options", Message: "một lựa chọn quá dài"}
		}
		if o.Correct {
			correct++
		}
		options = append(options, o)
	}

	switch {
	case len(options) < 2:
		return in, domain.InvalidInput{Field: "options", Message: "cần ít nhất 2 lựa chọn"}
	case len(options) > maxOptions:
		return in, domain.InvalidInput{Field: "options", Message: "quá nhiều lựa chọn"}
	case correct == 0:
		return in, domain.InvalidInput{Field: "options", Message: "phải đánh dấu ít nhất 1 đáp án đúng"}
	case correct == len(options):
		// Every option correct means the question cannot be got wrong, which is
		// nearly always a forgotten tick rather than a deliberate question.
		return in, domain.InvalidInput{Field: "options", Message: "không thể đánh dấu tất cả là đúng"}
	}
	in.Options = options
	return in, nil
}
