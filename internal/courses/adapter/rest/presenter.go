package rest

import (
	"time"

	"github.com/devforge/be/internal/courses/domain"
)

type courseSummary struct {
	ID           int64      `json:"id"`
	Slug         string     `json:"slug"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	ImageURL     *string    `json:"image_url"`
	Level        string     `json:"level"`
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
	TaskCount       int    `json:"task_count"`
	Points          int    `json:"points"`
}

type courseDetail struct {
	courseSummary
	Enrolled bool          `json:"enrolled"`
	Labs     []labResponse `json:"labs"`
}

func newDetail(c *domain.Course) courseDetail {
	labs := make([]labResponse, len(c.Labs))
	for i, l := range c.Labs {
		labs[i] = labResponse{
			ID:              l.ID,
			Slug:            l.Slug,
			Title:           l.Title,
			DescriptionMD:   l.DescriptionMD,
			DurationMinutes: l.DurationMinutes,
			OrderIdx:        l.OrderIdx,
			TaskCount:       l.TaskCount,
			Points:          l.Points,
		}
	}
	return courseDetail{
		courseSummary: newSummary(*c),
		Enrolled:      c.Enrolled,
		Labs:          labs,
	}
}

type reviewResponse struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	ContentMD string `json:"content_md"`
	OrderIdx  int    `json:"order_idx"`
}

func newReviewList(rs []domain.Review) []reviewResponse {
	out := make([]reviewResponse, len(rs))
	for i, r := range rs {
		out[i] = reviewResponse{ID: r.ID, Title: r.Title, ContentMD: r.ContentMD, OrderIdx: r.OrderIdx}
	}
	return out
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
