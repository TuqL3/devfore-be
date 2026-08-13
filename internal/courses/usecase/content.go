package usecase

import (
	"bytes"
	"context"
	"encoding/json"
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
	// A scenario is a catalogue of steps with a few numbers each; a goal is a
	// handful of clauses. Both are pasted as JSON, so the ceiling is against a
	// paste that went wrong rather than against a lab anybody would write.
	maxSimScenario = 20000
	maxSimGoal     = 4000
	// A break script and the shell that stands the service up are both bounded
	// like check_script is: long enough for a real setup, short enough that a
	// paste accident is refused rather than stored.
	maxBreakScript   = 8000
	maxIncidentSetup = 8000
	// Requests per second an outage is assumed to hurt. The ceiling is against a
	// typo — a drill claiming a million rps a second turns the cost counter into
	// noise, which is the one thing that panel exists to avoid.
	maxRPS = 100000
)

// cleanJSON bounds a pasted JSON object and refuses anything that is not one.
// Only the shape is checked here: what a valid scenario or goal contains is the
// simulator's business, and this module deliberately does not import it. An
// author who pastes a well-formed object with the wrong keys finds out when they
// run it, which is one screen away.
func cleanJSON(raw []byte, field string, max int) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	// An empty box and `{}` are the same intent — nothing here — and both are
	// stored as the column's own empty value by the repository.
	if len(trimmed) == 0 {
		return nil, nil
	}
	if len(trimmed) > max {
		return nil, domain.InvalidInput{Field: field, Message: "nội dung JSON quá dài"}
	}
	var obj map[string]any
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, domain.InvalidInput{Field: field, Message: "JSON không hợp lệ"}
	}
	return trimmed, nil
}

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

	scenario, err := cleanJSON(in.SimScenario, "sim_scenario", maxSimScenario)
	if err != nil {
		return in, err
	}
	in.SimScenario = scenario

	// A lab runs in a container or it simulates a pipeline, never both. The
	// database says the same thing, but a constraint violation surfaces as a
	// server error — an author picking an image on a sim lab has made an ordinary
	// mistake and deserves to be told which field it is in.
	if in.LabImageID != nil && len(in.SimScenario) > 0 {
		return in, domain.InvalidInput{
			Field:   "sim_scenario",
			Message: "một lab chỉ có thể là lab container hoặc lab mô phỏng, không thể cả hai",
		}
	}

	in.IncidentSetup = strings.TrimSpace(in.IncidentSetup)
	if len(in.IncidentSetup) > maxIncidentSetup {
		return in, domain.InvalidInput{Field: "incident_setup", Message: "script quá dài"}
	}
	// A sim lab has no container, so there is nothing for a setup script to run
	// in and nothing for a scenario to break. Refused here rather than left to
	// confuse an author whose script silently never runs.
	if len(in.SimScenario) > 0 && in.IncidentSetup != "" {
		return in, domain.InvalidInput{
			Field:   "incident_setup",
			Message: "lab mô phỏng không có container để dựng dịch vụ",
		}
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
		in.Kind != domain.KindCommand && in.Kind != domain.KindSim:
		return in, domain.InvalidInput{Field: "kind", Message: "loại nhiệm vụ không hợp lệ"}
	}

	goal, err := cleanJSON(in.SimGoal, "sim_goal", maxSimGoal)
	if err != nil {
		return in, err
	}
	in.SimGoal = goal

	if in.Kind == domain.KindSim {
		// Graded from the run the student produced, so nothing that grades against
		// a container means anything here.
		in.Options = nil
		in.CheckScript = ""
		in.ExpectedCommands = ""
		// An empty goal is allowed to be saved and refused at grading time. An
		// author writes the scenario, the task and the goal in three passes, and
		// blocking the save would mean holding the whole task hostage to the last
		// of them.
		return in, nil
	}

	// Every other kind is graded without a run, so a goal left over from a task
	// that used to be a sim task is dead weight that would come back to life if
	// it ever switched back.
	in.SimGoal = nil

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

// maxReviewMD is the same ceiling a lab's instructions get: both are one screen
// of markdown an author writes by hand.
const maxReviewMD = maxDescriptionMD

func (c *Courses) AdminReviews(ctx context.Context, courseID int64) ([]domain.Review, error) {
	// Reading the course first turns a bad id into a 404 rather than an empty
	// list, which reads as "this course has no notes yet".
	if _, err := c.repo.ByID(ctx, courseID); err != nil {
		return nil, err
	}
	return c.repo.Reviews(ctx, courseID)
}

func (c *Courses) CreateReview(ctx context.Context, courseID int64, in domain.ReviewInput) (*domain.Review, error) {
	if _, err := c.repo.ByID(ctx, courseID); err != nil {
		return nil, err
	}
	in, err := cleanReview(in)
	if err != nil {
		return nil, err
	}
	return c.repo.CreateReview(ctx, courseID, in)
}

func (c *Courses) UpdateReview(ctx context.Context, reviewID int64, in domain.ReviewInput) (*domain.Review, error) {
	in, err := cleanReview(in)
	if err != nil {
		return nil, err
	}
	return c.repo.UpdateReview(ctx, reviewID, in)
}

func (c *Courses) DeleteReview(ctx context.Context, reviewID int64) error {
	return c.repo.DeleteReview(ctx, reviewID)
}

// cleanReview trims and bounds. The body is markdown rendered on a page anyone
// signed in can read, so the length is a real limit rather than a formality —
// but the markup itself is left alone, because the reader renders it as text.
func cleanReview(in domain.ReviewInput) (domain.ReviewInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.ContentMD = strings.TrimSpace(in.ContentMD)

	switch {
	case in.Title == "":
		return in, domain.InvalidInput{Field: "title", Message: "tiêu đề không được để trống"}
	case len(in.Title) > maxTitle:
		return in, domain.InvalidInput{Field: "title", Message: "tiêu đề quá dài"}
	case len(in.ContentMD) > maxReviewMD:
		return in, domain.InvalidInput{Field: "content_md", Message: "nội dung quá dài"}
	case in.OrderIdx < 0:
		return in, domain.InvalidInput{Field: "order_idx", Message: "thứ tự không được âm"}
	}
	return in, nil
}

// ── War Room scenarios ─────────────────────────────────────────────────────

// AdminDrills needs no validation and no ownership check beyond the admin role
// the route already carries: it is a read of everything.
func (c *Courses) AdminDrills(ctx context.Context) ([]domain.Drill, error) {
	return c.repo.AdminDrills(ctx)
}

// CreateDrill makes a challenge that belongs to no course.
//
// Runs the same cleanLab as an ordinary lab: a drill is a lab, and every rule
// there — slug shape, duration bounds, the image-or-scenario exclusion — applies
// to it unchanged. The setup script is required here and only here, because it
// is what puts the lab in the War Room list at all; a challenge saved without
// one would vanish the moment it was created.
func (c *Courses) CreateDrill(ctx context.Context, in domain.LabInput) (*domain.Lab, error) {
	clean, err := c.cleanLab(ctx, in)
	if err != nil {
		return nil, err
	}
	if clean.IncidentSetup == "" {
		return nil, domain.InvalidInput{
			Field:   "incident_setup",
			Message: "thử thách phải có script dựng dịch vụ",
		}
	}
	return c.repo.CreateDrill(ctx, clean)
}

// SetDrillStatus opens or closes a War Room challenge.
//
// Publishing one with nothing left to draw is refused: it would sit in the list
// and then hand a student a container in which nothing is broken, which reads as
// the platform being broken rather than the drill being unfinished. Drafting is
// never refused — taking something down has to work whatever state it is in.
func (c *Courses) SetDrillStatus(ctx context.Context, labID int64, status string) error {
	if status != domain.DrillDraft && status != domain.DrillPublished {
		return domain.InvalidInput{Field: "status", Message: "trạng thái không hợp lệ"}
	}
	// Reading the lab first turns a bad id into a 404 rather than a write that
	// silently matches no rows.
	if _, err := c.repo.AdminLab(ctx, labID); err != nil {
		return err
	}
	if status == domain.DrillPublished {
		n, err := c.repo.ActiveIncidentCount(ctx, labID)
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNoActiveIncident
		}
	}
	return c.repo.SetDrillStatus(ctx, labID, status)
}

func (c *Courses) AdminIncidents(ctx context.Context, labID int64) ([]domain.Incident, error) {
	// Reading the lab first turns a bad id into a 404 rather than an empty list
	// that reads as "this lab has no scenarios".
	if _, err := c.repo.AdminLab(ctx, labID); err != nil {
		return nil, err
	}
	return c.repo.AdminIncidents(ctx, labID)
}

func (c *Courses) CreateIncident(ctx context.Context, labID int64, in domain.IncidentInput) (*domain.Incident, error) {
	clean, err := cleanIncident(in)
	if err != nil {
		return nil, err
	}
	return c.repo.CreateIncident(ctx, labID, clean)
}

func (c *Courses) UpdateIncident(ctx context.Context, incidentID int64, in domain.IncidentInput) (*domain.Incident, error) {
	clean, err := cleanIncident(in)
	if err != nil {
		return nil, err
	}
	return c.repo.UpdateIncident(ctx, incidentID, clean)
}

func (c *Courses) DeleteIncident(ctx context.Context, incidentID int64) error {
	return c.repo.DeleteIncident(ctx, incidentID)
}

// cleanIncident needs no repository: everything it checks is in the input. The
// break script is bounded but not otherwise inspected — it is shell, and
// guessing at valid shell here would refuse things that work.
func cleanIncident(in domain.IncidentInput) (domain.IncidentInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.BreakScript = strings.TrimSpace(in.BreakScript)
	in.RevealMD = strings.TrimSpace(in.RevealMD)

	switch {
	case in.Title == "":
		return in, domain.InvalidInput{Field: "title", Message: "tiêu đề không được để trống"}
	case len(in.Title) > maxTitle:
		return in, domain.InvalidInput{Field: "title", Message: "tiêu đề quá dài"}
	case len(in.BreakScript) > maxBreakScript:
		return in, domain.InvalidInput{Field: "break_script", Message: "script quá dài"}
	case len(in.RevealMD) > maxDescriptionMD:
		return in, domain.InvalidInput{Field: "reveal_md", Message: "nội dung quá dài"}
	case in.RPS < 0 || in.RPS > maxRPS:
		return in, domain.InvalidInput{Field: "rps", Message: "rps phải từ 0 đến 100000"}
	}

	// The one invariant worth enforcing: an active scenario with no script
	// starts a drill in which nothing is broken, and the student hunts a fault
	// that does not exist. Saving it retired is how a draft gets written.
	if in.Active && in.BreakScript == "" {
		return in, domain.InvalidInput{
			Field:   "break_script",
			Message: "kịch bản đang bật thì phải có script phá — hoặc tắt nó đi",
		}
	}
	return in, nil
}
