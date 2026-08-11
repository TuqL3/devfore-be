package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/devforge/be/internal/courses/domain"
)

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

func (r *CourseRepo) ListPublished(ctx context.Context, f domain.CourseFilter) ([]domain.Course, error) {
	// Seeded with a constant so every filter below can append unconditionally,
	// including none of them.
	where := []string{"true"}
	args := []any{}
	// Drafts are unfinished courses, not hidden ones: they are only ever listed
	// for an admin, and the caller has to ask for them explicitly.
	if !f.IncludeDrafts {
		where = append(where, "c.status = 'published'")
	}
	if f.Level != "" {
		where = append(where, "c.level = ?")
		args = append(args, f.Level)
	}
	if f.Query != "" {
		where = append(where, "(c.title ILIKE ? OR c.description ILIKE ?)")
		like := "%" + f.Query + "%"
		args = append(args, like, like)
	}

	var rows []courseRow
	// Easiest first, newest first within a level. Sorting by date alone put
	// whatever was published last at the top, so a beginner landed on the
	// hardest course on the site — the list is a path through the material, not
	// a feed. LEFT JOIN so a course whose level is not in the table still shows,
	// after the ones that are.
	err := r.db.WithContext(ctx).Raw(
		`SELECT `+courseCols+` FROM courses c
		 LEFT JOIN levels lv ON lv.slug = c.level
		 WHERE `+strings.Join(where, " AND ")+`
		 ORDER BY lv.rank NULLS LAST, c.published_at DESC NULLS LAST, c.id DESC`,
		args...,
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

func (r *CourseRepo) ListLevels(ctx context.Context) ([]domain.Level, error) {
	var rows []struct {
		Slug        string
		Label       string
		Hint        string
		Rank        int
		CourseCount int64
	}
	err := r.db.WithContext(ctx).Raw(
		`SELECT lv.slug, lv.label, lv.hint, lv.rank,
		        (SELECT count(*) FROM courses c
		          WHERE c.level = lv.slug AND c.status = 'published') AS course_count
		 FROM levels lv ORDER BY lv.rank`,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Level, len(rows))
	for i, row := range rows {
		out[i] = domain.Level(row)
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
		// The scenario itself is not on the listing — only whether there is one.
		// A course page showing ten labs has no use for ten catalogues, and the
		// one screen that needs the answer only needs the bit.
		`SELECT l.id, l.slug, l.title, l.description_md, l.duration_minutes, l.order_idx,
			(l.sim_scenario IS NOT NULL)                                          AS is_sim,
			(SELECT count(*) FROM lab_tasks t WHERE t.lab_id = l.id)               AS task_count,
			COALESCE((SELECT sum(points) FROM lab_tasks t WHERE t.lab_id = l.id), 0) AS points
		 FROM labs l WHERE l.course_id = ? ORDER BY l.order_idx, l.id`, courseID,
	).Scan(&labs).Error
	return labs, err
}

// Drills are the War Room's list: every lab that has an incident scenario ready
// to hand out. They are not course material and are not reached through a course
// — the course row a lab points at is only a place for it to live — so nothing
// here filters on course status or enrolment.
//
// Having an active lab_incidents row is what makes a lab a drill, the same fact
// the runtime asks when it decides whether to break the container. One rule, one
// place it is written down.
func (r *CourseRepo) Drills(ctx context.Context) ([]domain.Lab, error) {
	labs := []domain.Lab{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT l.id, l.slug, l.title, l.description_md, l.duration_minutes, l.order_idx,
			false                                                                  AS is_sim,
			(SELECT count(*) FROM lab_tasks t WHERE t.lab_id = l.id)               AS task_count,
			COALESCE((SELECT sum(points) FROM lab_tasks t WHERE t.lab_id = l.id), 0) AS points
		   FROM labs l
		  WHERE EXISTS (SELECT 1 FROM lab_incidents i WHERE i.lab_id = l.id AND i.active)
		  ORDER BY l.order_idx, l.id`,
	).Scan(&labs).Error
	return labs, err
}

// DrillBySlug is LabBySlug without the course in the path, for the same reason:
// a drill is addressed by itself. Refuses a lab that is not a drill rather than
// answering for it, or this becomes a way to read any lab's page while skipping
// the enrolment its course asks for.
func (r *CourseRepo) DrillBySlug(ctx context.Context, slug string) (*domain.Lab, error) {
	var lab domain.Lab
	res := r.db.WithContext(ctx).Raw(
		`SELECT l.id, l.slug, l.title, l.description_md, l.duration_minutes, l.order_idx,
			false                                                                  AS is_sim,
			(SELECT count(*) FROM lab_tasks t WHERE t.lab_id = l.id)               AS task_count,
			COALESCE((SELECT sum(points) FROM lab_tasks t WHERE t.lab_id = l.id), 0) AS points
		   FROM labs l
		  WHERE l.slug = ?
		    AND EXISTS (SELECT 1 FROM lab_incidents i WHERE i.lab_id = l.id AND i.active)`,
		slug,
	).Scan(&lab)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrLabNotFound
	}
	return &lab, nil
}

// Lab slugs are unique across the whole table, but the lookup is still scoped to
// the course so a lab cannot be reached through the wrong course's URL.
//
// sim_scenario is the one authored field here that reaches the student, and only
// on this query rather than on the course listing beside it: a pipeline is
// written against the catalogue of steps and the runner count, so withholding
// them would leave the editor with nothing to offer. lab_image_id stays out —
// which image a lab runs is an authoring detail.
func (r *CourseRepo) LabBySlug(ctx context.Context, courseID int64, slug string) (*domain.Lab, error) {
	var lab domain.Lab
	res := r.db.WithContext(ctx).Raw(
		`SELECT l.id, l.slug, l.title, l.description_md, l.duration_minutes, l.order_idx,
			l.sim_scenario, (l.sim_scenario IS NOT NULL) AS is_sim,
			(SELECT count(*) FROM lab_tasks t WHERE t.lab_id = l.id)                 AS task_count,
			COALESCE((SELECT sum(points) FROM lab_tasks t WHERE t.lab_id = l.id), 0) AS points
		 FROM labs l WHERE l.course_id = ? AND l.slug = ?`, courseID, slug,
	).Scan(&lab)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrLabNotFound
	}
	return &lab, nil
}

// check_script stays out of the column list on purpose: it is the answer key.
// So does options.correct — the option text has to reach the student, the flag
// saying which one is right must not, so it is stripped in SQL rather than in
// Go, where a later refactor could forget.
func (r *CourseRepo) TasksByLab(ctx context.Context, labID int64) ([]domain.Task, error) {
	rows := []struct {
		ID           int64
		Title        string
		Hint         string
		Points       int
		OrderIdx     int
		Kind         string
		Options      []byte
		SingleAnswer bool
	}{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, title, hint, points, order_idx, kind,
		        COALESCE(
		          (SELECT jsonb_agg(o->'text') FROM jsonb_array_elements(options) o),
		          '[]'::jsonb
		        ) AS options,
		        -- How MANY options are right, reduced to one bit before it
		        -- leaves SQL: enough to pick a radio over a checkbox, never
		        -- enough to say which one.
		        (SELECT count(*) FROM jsonb_array_elements(options) o
		          WHERE (o->>'correct')::boolean) = 1 AS single_answer
		   FROM lab_tasks
		  WHERE lab_id = ? ORDER BY order_idx, id`, labID,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	tasks := make([]domain.Task, len(rows))
	for i, row := range rows {
		t := domain.Task{
			ID:           row.ID,
			Title:        row.Title,
			Hint:         row.Hint,
			Points:       row.Points,
			OrderIdx:     row.OrderIdx,
			Kind:         row.Kind,
			Options:      []string{},
			SingleAnswer: row.SingleAnswer,
		}
		if len(row.Options) > 0 {
			if err := json.Unmarshal(row.Options, &t.Options); err != nil {
				return nil, fmt.Errorf("đọc lựa chọn của nhiệm vụ %d: %w", row.ID, err)
			}
		}
		tasks[i] = t
	}
	return tasks, nil
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

func (r *CourseRepo) Unenroll(ctx context.Context, userID, courseID int64) error {
	return r.db.WithContext(ctx).Exec(
		`DELETE FROM enrollments WHERE user_id = ? AND course_id = ?`,
		userID, courseID,
	).Error
}

func (r *CourseRepo) IsEnrolled(ctx context.Context, userID, courseID int64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("enrollments").
		Where("user_id = ? AND course_id = ?", userID, courseID).Count(&n).Error
	return n > 0, err
}

func (r *CourseRepo) Leaderboard(ctx context.Context, courseID int64) ([]domain.LeaderRow, error) {
	rows := []domain.LeaderRow{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT u.username, u.avatar_url, cs.score, cs.labs_completed, cs.attempts, cs.updated_at
		 FROM course_scores cs
		 JOIN users u ON u.id = cs.user_id
		 WHERE cs.course_id = ?
		 ORDER BY cs.score DESC, u.username
		 LIMIT 50`, courseID,
	).Scan(&rows).Error
	return rows, err
}

// Enrolled lists the courses this student signed up for, most recently enrolled
// first, each with how far they have got. Drafts are kept in: unpublishing a
// course an admin is reworking should not make it vanish from the shelf of
// somebody who already started it.
func (r *CourseRepo) Enrolled(ctx context.Context, userID int64) ([]domain.Enrollment, error) {
	// Spelled out rather than embedding courseRow: gorm's Scan maps columns onto
	// named fields only, so an anonymous embed reads back as a zero course while
	// the fields beside it fill in perfectly — a result that looks like data.
	var rows []struct {
		ID            int64
		Slug          string
		Title         string
		Description   string
		ImageURL      *string
		Level         string
		Status        string
		PublishedAt   *time.Time
		UpdatedAt     time.Time
		LabCount      int
		StudentCount  int64
		Score         int
		LabsCompleted int
		EnrolledAt    time.Time
	}
	err := r.db.WithContext(ctx).Raw(
		`SELECT `+courseCols+`,
		        COALESCE(cs.score, 0)          AS score,
		        COALESCE(cs.labs_completed, 0) AS labs_completed,
		        e.created_at                   AS enrolled_at
		   FROM enrollments e
		   JOIN courses c ON c.id = e.course_id
		   LEFT JOIN course_scores cs ON cs.course_id = c.id AND cs.user_id = e.user_id
		  WHERE e.user_id = ?
		  ORDER BY e.created_at DESC, c.id DESC`, userID,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Enrollment, len(rows))
	for i, row := range rows {
		course := courseRow{
			ID:           row.ID,
			Slug:         row.Slug,
			Title:        row.Title,
			Description:  row.Description,
			ImageURL:     row.ImageURL,
			Level:        row.Level,
			Status:       row.Status,
			PublishedAt:  row.PublishedAt,
			UpdatedAt:    row.UpdatedAt,
			LabCount:     row.LabCount,
			StudentCount: row.StudentCount,
		}.toDomain()
		// Every course in this list is one they are enrolled in, by definition of
		// the query — saying so saves the caller a second lookup to find out.
		course.Enrolled = true
		out[i] = domain.Enrollment{
			Course:        course,
			Score:         row.Score,
			LabsCompleted: row.LabsCompleted,
			EnrolledAt:    row.EnrolledAt,
		}
	}
	return out, nil
}
