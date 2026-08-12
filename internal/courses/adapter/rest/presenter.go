package rest

import (
	"encoding/json"
	"time"

	"github.com/devforge/be/internal/courses/domain"
)

type courseSummary struct {
	ID          int64   `json:"id"`
	Slug        string  `json:"slug"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	ImageURL    *string `json:"image_url"`
	Level       string  `json:"level"`
	// Public listings only ever contain published courses, so this is here for
	// the admin one, where telling a draft from a live course is the point.
	Status       string     `json:"status"`
	LabCount     int        `json:"lab_count"`
	StudentCount int64      `json:"student_count"`
	PublishedAt  *time.Time `json:"published_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func newSummary(c domain.Course) courseSummary {
	return courseSummary{
		ID:           c.ID,
		Slug:         c.Slug,
		Title:        c.Title,
		Description:  c.Description,
		ImageURL:     c.ImageURL,
		Level:        c.Level,
		Status:       c.Status,
		LabCount:     c.LabCount,
		StudentCount: c.StudentCount,
		PublishedAt:  c.PublishedAt,
		UpdatedAt:    c.UpdatedAt,
	}
}

func newSummaryList(cs []domain.Course) []courseSummary {
	out := make([]courseSummary, len(cs))
	for i, c := range cs {
		out[i] = newSummary(c)
	}
	return out
}

type labResponse struct {
	ID              int64  `json:"id"`
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	DescriptionMD   string `json:"description_md"`
	DurationMinutes int    `json:"duration_minutes"`
	OrderIdx        int    `json:"order_idx"`
	// Whether starting this lab creates a container. On the listing so the dialog
	// in front of Start stops promising one for a lab that simulates instead.
	IsSim     bool `json:"is_sim"`
	TaskCount int  `json:"task_count"`
	Points    int  `json:"points"`
}

func newLab(l *domain.Lab) labResponse {
	return labResponse{
		ID:              l.ID,
		Slug:            l.Slug,
		Title:           l.Title,
		DescriptionMD:   l.DescriptionMD,
		DurationMinutes: l.DurationMinutes,
		OrderIdx:        l.OrderIdx,
		IsSim:           l.IsSim,
		TaskCount:       l.TaskCount,
		Points:          l.Points,
	}
}

type courseDetail struct {
	courseSummary
	Enrolled bool          `json:"enrolled"`
	Labs     []labResponse `json:"labs"`
}

func newDetail(c *domain.Course) courseDetail {
	labs := make([]labResponse, len(c.Labs))
	for i := range c.Labs {
		labs[i] = newLab(&c.Labs[i])
	}
	return courseDetail{
		courseSummary: newSummary(*c),
		Enrolled:      c.Enrolled,
		Labs:          labs,
	}
}

type taskResponse struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Hint     string `json:"hint"`
	Points   int    `json:"points"`
	OrderIdx int    `json:"order_idx"`
	Kind     string `json:"kind"`
	// Option text only. Whether an option is the right one is decided on the
	// server when the answer is submitted; sending it here would put the answer
	// key in the page the question is asked on.
	Options []string `json:"options"`
	// Whether the question takes one answer or several. How many, not which.
	SingleAnswer bool `json:"single_answer"`
}

// labDetail is the lab screen. sim_scenario sits here and not on labResponse
// because the course listing has no use for it, and because this is the one
// response a student needs it in: the catalogue of steps and the runner count
// are what a pipeline is written against.
//
// The pass condition is a different matter and has no field here at all —
// domain.Task carries no goal, the same way it carries no check script, so there
// is nothing for a later edit to accidentally wire up.
type labDetail struct {
	labResponse
	SimScenario json.RawMessage `json:"sim_scenario"`
	Tasks       []taskResponse  `json:"tasks"`
}

func newLabDetail(l *domain.Lab) labDetail {
	tasks := make([]taskResponse, len(l.Tasks))
	for i, t := range l.Tasks {
		options := t.Options
		if options == nil {
			options = []string{}
		}
		tasks[i] = taskResponse{
			ID:           t.ID,
			Title:        t.Title,
			Hint:         t.Hint,
			Points:       t.Points,
			OrderIdx:     t.OrderIdx,
			Kind:         t.Kind,
			Options:      options,
			SingleAnswer: t.SingleAnswer,
		}
	}
	return labDetail{
		labResponse: newLab(l),
		SimScenario: rawJSON(l.SimScenario),
		Tasks:       tasks,
	}
}

type reviewResponse struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	ContentMD string `json:"content_md"`
	OrderIdx  int    `json:"order_idx"`
}

func newReview(r domain.Review) reviewResponse {
	return reviewResponse{ID: r.ID, Title: r.Title, ContentMD: r.ContentMD, OrderIdx: r.OrderIdx}
}

func newReviewList(rs []domain.Review) []reviewResponse {
	out := make([]reviewResponse, len(rs))
	for i, r := range rs {
		out[i] = newReview(r)
	}
	return out
}

// reviewInput is the same shape coming back the other way. order_idx is a
// pointer-free int: the create path ignores it and appends, so a form that does
// not send one gets the end of the list rather than position zero.
type reviewInput struct {
	Title     string `json:"title"`
	ContentMD string `json:"content_md"`
	OrderIdx  int    `json:"order_idx"`
}

func (in reviewInput) toDomain() domain.ReviewInput {
	return domain.ReviewInput{
		Title:     in.Title,
		ContentMD: in.ContentMD,
		OrderIdx:  in.OrderIdx,
	}
}

type leaderRow struct {
	Rank          int       `json:"rank"`
	Username      string    `json:"username"`
	AvatarURL     *string   `json:"avatar_url"`
	Score         int       `json:"score"`
	LabsCompleted int       `json:"labs_completed"`
	Attempts      int       `json:"attempts"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func newLeaderboard(rows []domain.LeaderRow) []leaderRow {
	out := make([]leaderRow, len(rows))
	for i, r := range rows {
		out[i] = leaderRow{
			Rank:          i + 1,
			Username:      r.Username,
			AvatarURL:     r.AvatarURL,
			Score:         r.Score,
			LabsCompleted: r.LabsCompleted,
			Attempts:      r.Attempts,
			UpdatedAt:     r.UpdatedAt,
		}
	}
	return out
}

type levelResponse struct {
	Slug        string `json:"slug"`
	Label       string `json:"label"`
	Hint        string `json:"hint"`
	Rank        int    `json:"rank"`
	CourseCount int64  `json:"course_count"`
}

func newLevelList(levels []domain.Level) []levelResponse {
	out := make([]levelResponse, len(levels))
	for i, l := range levels {
		out[i] = levelResponse(l)
	}
	return out
}

// enrollmentResponse is a course summary plus this student's progress in it.
// Flattened rather than nested, because every field on it is about the same
// thing from the reader's point of view: one row on their profile.
type enrollmentResponse struct {
	courseSummary
	Score         int       `json:"score"`
	LabsCompleted int       `json:"labs_completed"`
	EnrolledAt    time.Time `json:"enrolled_at"`
}

func newEnrollmentList(es []domain.Enrollment) []enrollmentResponse {
	out := make([]enrollmentResponse, len(es))
	for i, e := range es {
		out[i] = enrollmentResponse{
			courseSummary: newSummary(e.Course),
			Score:         e.Score,
			LabsCompleted: e.LabsCompleted,
			EnrolledAt:    e.EnrolledAt,
		}
	}
	return out
}
