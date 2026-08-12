package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/devforge/be/internal/courses/domain"
)

// labCols is the admin view of a lab: the public one leaves out the image, and
// the counts are read the same way the course page reads them.
const labCols = `l.id, l.slug, l.title, l.description_md, l.duration_minutes, l.order_idx,
	l.lab_image_id, l.sim_scenario, l.incident_setup,
	(SELECT count(*) FROM lab_tasks t WHERE t.lab_id = l.id)                 AS task_count,
	COALESCE((SELECT sum(points) FROM lab_tasks t WHERE t.lab_id = l.id), 0) AS points,
	(SELECT count(*) FROM lab_incidents i WHERE i.lab_id = l.id AND i.active) AS incident_count`

func (r *CourseRepo) AdminLabs(ctx context.Context, courseID int64) ([]domain.Lab, error) {
	labs := []domain.Lab{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT `+labCols+` FROM labs l WHERE l.course_id = ? ORDER BY l.order_idx, l.id`,
		courseID,
	).Scan(&labs).Error
	return labs, err
}

func (r *CourseRepo) AdminLab(ctx context.Context, labID int64) (*domain.Lab, error) {
	var lab domain.Lab
	res := r.db.WithContext(ctx).Raw(
		`SELECT `+labCols+` FROM labs l WHERE l.id = ?`, labID,
	).Scan(&lab)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrLabNotFound
	}
	return &lab, nil
}

// CreateLab puts the new lab last. The position is derived rather than sent:
// an author adding a lab means "after the others", and a client computing that
// from a stale list would collide with itself.
func (r *CourseRepo) CreateLab(ctx context.Context, courseID int64, in domain.LabInput) (*domain.Lab, error) {
	var id int64
	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO labs (course_id, slug, title, description_md, duration_minutes, lab_image_id,
		                   sim_scenario, incident_setup, order_idx)
		 VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?,
		         COALESCE((SELECT max(order_idx) + 1 FROM labs WHERE course_id = ?), 0))
		 RETURNING id`,
		courseID, in.Slug, in.Title, in.DescriptionMD, in.DurationMinutes, in.LabImageID,
		nullableJSON(in.SimScenario), in.IncidentSetup, courseID,
	).Scan(&id).Error
	if isSlugTaken(err) {
		return nil, domain.ErrLabSlugTaken
	}
	if err != nil {
		return nil, err
	}
	return r.AdminLab(ctx, id)
}

func (r *CourseRepo) UpdateLab(ctx context.Context, labID int64, in domain.LabInput) (*domain.Lab, error) {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE labs SET slug = ?, title = ?, description_md = ?, duration_minutes = ?,
		        lab_image_id = ?, sim_scenario = ?::jsonb, incident_setup = ?,
		        order_idx = ?, updated_at = now()
		  WHERE id = ?`,
		in.Slug, in.Title, in.DescriptionMD, in.DurationMinutes, in.LabImageID,
		nullableJSON(in.SimScenario), in.IncidentSetup, in.OrderIdx, labID,
	)
	if isSlugTaken(res.Error) {
		return nil, domain.ErrLabSlugTaken
	}
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrLabNotFound
	}
	return r.AdminLab(ctx, labID)
}

// DeleteLab takes its tasks with it through the cascade, and with them every
// completion row that pointed at those tasks.
func (r *CourseRepo) DeleteLab(ctx context.Context, labID int64) error {
	res := r.db.WithContext(ctx).Exec(`DELETE FROM labs WHERE id = ?`, labID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrLabNotFound
	}
	return nil
}

// adminTaskRow keeps options as raw JSON on the way out of the database, which
// is the only place the answer key and the option text travel together.
type adminTaskRow struct {
	ID               int64
	Title            string
	Hint             string
	Points           int
	OrderIdx         int
	Kind             string
	CheckScript      string
	Options          []byte
	ExpectedCommands string
	SimGoal          []byte
}

func (row adminTaskRow) toDomain() (domain.AdminTask, error) {
	t := domain.AdminTask{
		ID:               row.ID,
		Title:            row.Title,
		Hint:             row.Hint,
		Points:           row.Points,
		OrderIdx:         row.OrderIdx,
		Kind:             row.Kind,
		CheckScript:      row.CheckScript,
		Options:          []domain.Option{},
		ExpectedCommands: row.ExpectedCommands,
		SimGoal:          row.SimGoal,
	}
	if len(row.Options) > 0 {
		if err := json.Unmarshal(row.Options, &t.Options); err != nil {
			return t, fmt.Errorf("đọc lựa chọn của nhiệm vụ %d: %w", row.ID, err)
		}
	}
	return t, nil
}

const adminTaskCols = `id, title, hint, points, order_idx, kind, check_script, options,
	expected_commands, sim_goal`

// AdminTasks is the one query that reads check_script and the correct flags out
// of the database. It answers the admin editor, which has to show an author what
// they wrote.
func (r *CourseRepo) AdminTasks(ctx context.Context, labID int64) ([]domain.AdminTask, error) {
	rows := []adminTaskRow{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT `+adminTaskCols+` FROM lab_tasks
		  WHERE lab_id = ? ORDER BY order_idx, id`, labID,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	tasks := make([]domain.AdminTask, len(rows))
	for i, row := range rows {
		if tasks[i], err = row.toDomain(); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

func (r *CourseRepo) AdminTask(ctx context.Context, taskID int64) (*domain.AdminTask, error) {
	var row adminTaskRow
	res := r.db.WithContext(ctx).Raw(
		`SELECT `+adminTaskCols+` FROM lab_tasks WHERE id = ?`, taskID,
	).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrTaskNotFound
	}
	t, err := row.toDomain()
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// optionsJSON always produces an array. A nil slice marshals to `null`, and the
// grading queries read options with jsonb_array_elements, which errors on a
// scalar — a task saved with no options would take down every read of its lab,
// including the public one.
func optionsJSON(options []domain.Option) (string, error) {
	if options == nil {
		options = []domain.Option{}
	}
	b, err := json.Marshal(options)
	return string(b), err
}

// nullableJSON hands the driver a NULL for an empty scenario rather than an empty
// string, which `?::jsonb` would refuse. NULL is also the value every other query
// reads to mean "container lab", so an author clearing the box turns the lab back
// into one rather than leaving it in a third state.
func nullableJSON(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

// goalJSON is the same idea for a column that is NOT NULL. A task with no goal
// stores `{}`, which is what grading refuses — an unfinished task, said plainly.
func goalJSON(raw []byte) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}

func (r *CourseRepo) CreateTask(ctx context.Context, labID int64, in domain.TaskInput) (*domain.AdminTask, error) {
	options, err := optionsJSON(in.Options)
	if err != nil {
		return nil, err
	}
	var id int64
	err = r.db.WithContext(ctx).Raw(
		`INSERT INTO lab_tasks (lab_id, title, hint, points, kind, check_script, options,
		                        expected_commands, sim_goal, order_idx)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb,
		         COALESCE((SELECT max(order_idx) + 1 FROM lab_tasks WHERE lab_id = ?), 0))
		 RETURNING id`,
		labID, in.Title, in.Hint, in.Points, in.Kind, in.CheckScript, options,
		in.ExpectedCommands, goalJSON(in.SimGoal), labID,
	).Scan(&id).Error
	if err != nil {
		return nil, err
	}
	return r.AdminTask(ctx, id)
}

func (r *CourseRepo) UpdateTask(ctx context.Context, taskID int64, in domain.TaskInput) (*domain.AdminTask, error) {
	options, err := optionsJSON(in.Options)
	if err != nil {
		return nil, err
	}
	res := r.db.WithContext(ctx).Exec(
		`UPDATE lab_tasks SET title = ?, hint = ?, points = ?, kind = ?,
		        check_script = ?, options = ?, expected_commands = ?, sim_goal = ?::jsonb,
		        order_idx = ?
		  WHERE id = ?`,
		in.Title, in.Hint, in.Points, in.Kind, in.CheckScript, options,
		in.ExpectedCommands, goalJSON(in.SimGoal), in.OrderIdx, taskID,
	)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrTaskNotFound
	}
	return r.AdminTask(ctx, taskID)
}

// DeleteTask drops the completions that referenced it along the way, which is
// the cascade doing what an author asked for: a deleted question is not one
// anybody has answered.
func (r *CourseRepo) DeleteTask(ctx context.Context, taskID int64) error {
	res := r.db.WithContext(ctx).Exec(`DELETE FROM lab_tasks WHERE id = ?`, taskID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

// LabImages lists what an author can pin a lab to. Inactive images stay in the
// list: a lab already using one has to keep showing which, and hiding it would
// make the form silently change the lab on the next save.
func (r *CourseRepo) LabImages(ctx context.Context) ([]domain.LabImage, error) {
	images := []domain.LabImage{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT id, name, tag, COALESCE(description, '') AS description, active
		   FROM lab_images ORDER BY active DESC, name, tag`,
	).Scan(&images).Error
	return images, err
}

func (r *CourseRepo) LabImageExists(ctx context.Context, id int64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(
		`SELECT count(*) FROM lab_images WHERE id = ?`, id,
	).Scan(&n).Error
	return n > 0, err
}

// AdminReview reads one revision note. Same columns as the public read — there
// is nothing in a review that only an author may see — but addressed by id,
// because that is what the editor holds after a save.
func (r *CourseRepo) AdminReview(ctx context.Context, reviewID int64) (*domain.Review, error) {
	var review domain.Review
	res := r.db.WithContext(ctx).Raw(
		`SELECT id, title, content_md, order_idx FROM reviews WHERE id = ?`, reviewID,
	).Scan(&review)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrReviewNotFound
	}
	return &review, nil
}

// CreateReview appends to the end of the course's list. Same as a new lab or a
// new task: an author writing a note is adding to what is there, and picking a
// position for them is one decision they did not ask to make.
func (r *CourseRepo) CreateReview(ctx context.Context, courseID int64, in domain.ReviewInput) (*domain.Review, error) {
	var id int64
	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO reviews (course_id, title, content_md, order_idx)
		 VALUES (?, ?, ?,
		         COALESCE((SELECT max(order_idx) + 1 FROM reviews WHERE course_id = ?), 0))
		 RETURNING id`,
		courseID, in.Title, in.ContentMD, courseID,
	).Scan(&id).Error
	if err != nil {
		return nil, err
	}
	return r.AdminReview(ctx, id)
}

func (r *CourseRepo) UpdateReview(ctx context.Context, reviewID int64, in domain.ReviewInput) (*domain.Review, error) {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE reviews SET title = ?, content_md = ?, order_idx = ? WHERE id = ?`,
		in.Title, in.ContentMD, in.OrderIdx, reviewID,
	)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrReviewNotFound
	}
	return r.AdminReview(ctx, reviewID)
}

func (r *CourseRepo) DeleteReview(ctx context.Context, reviewID int64) error {
	res := r.db.WithContext(ctx).Exec(`DELETE FROM reviews WHERE id = ?`, reviewID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrReviewNotFound
	}
	return nil
}

// ── War Room scenarios ─────────────────────────────────────────────────────
//
// Retired rows stay in every read here. The admin screen has to show them: a
// scenario is retired rather than deleted precisely because finished reports
// still point at it, and hiding it would leave an author wondering where it
// went and writing a duplicate.

// AdminDrills is the War Room's own admin list: every lab that carries a
// scenario, whether or not any of them is active.
//
// Retired-only labs stay in it. They are exactly the ones an author is looking
// for — a lab that dropped out of War Room because its last scenario was turned
// off is invisible everywhere else, and the student-facing Drills query filters
// them out by design.
//
// A lab with a setup script but no scenario yet is in the list too, and that is
// what makes the section usable at all: writing the script is how a lab is filed
// under War Room, and a list that waited for the first scenario would be a list
// you could never add the first scenario from.
//
// LEFT JOIN, and the course fields come back empty for a challenge created in
// War Room: since 000028 a drill has no course. The join stays only for the
// older ones that were made inside a course before that was possible.
func (r *CourseRepo) AdminDrills(ctx context.Context) ([]domain.Drill, error) {
	drills := []domain.Drill{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT l.id, l.slug, l.title, l.duration_minutes, l.lab_image_id,
		        l.incident_setup, l.drill_status AS status,
		        c.id AS course_id, COALESCE(c.title, '') AS course_title,
		        (SELECT count(*) FROM lab_incidents i WHERE i.lab_id = l.id AND i.active) AS incident_count,
		        (SELECT count(*) FROM lab_incidents i WHERE i.lab_id = l.id)              AS scenario_count
		   FROM labs l
		   LEFT JOIN courses c ON c.id = l.course_id
		  WHERE EXISTS (SELECT 1 FROM lab_incidents i WHERE i.lab_id = l.id)
		     OR l.incident_setup <> ''
		  ORDER BY l.id`,
	).Scan(&drills).Error
	return drills, err
}

// CreateDrill makes a War Room challenge, which is a lab belonging to no course.
//
// Separate from CreateLab rather than a nil argument to it: the ordering that
// one derives (`max(order_idx) + 1 FROM labs WHERE course_id = ?`) has no
// meaning without a course, and a challenge has no siblings to be ordered
// against anyway.
func (r *CourseRepo) CreateDrill(ctx context.Context, in domain.LabInput) (*domain.Lab, error) {
	var id int64
	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO labs (course_id, slug, title, description_md, duration_minutes,
		                   lab_image_id, incident_setup, order_idx)
		 VALUES (NULL, ?, ?, ?, ?, ?, ?, 0)
		 RETURNING id`,
		in.Slug, in.Title, in.DescriptionMD, in.DurationMinutes, in.LabImageID,
		in.IncidentSetup,
	).Scan(&id).Error
	if isSlugTaken(err) {
		return nil, domain.ErrLabSlugTaken
	}
	if err != nil {
		return nil, err
	}
	return r.AdminLab(ctx, id)
}

// SetDrillStatus is its own write rather than a field of the lab form: taking a
// challenge down is a decision about the challenge, not an edit of the lab, and
// folding it into the form would mean every unrelated save re-asserted it.
func (r *CourseRepo) SetDrillStatus(ctx context.Context, labID int64, status string) error {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE labs SET drill_status = ?, updated_at = now() WHERE id = ?`, status, labID,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrLabNotFound
	}
	return nil
}

// ActiveIncidentCount answers the one question publishing has to ask: is there
// any fault left to draw. Read separately rather than off the list so the check
// runs against the row as it is now, not as some earlier page render saw it.
func (r *CourseRepo) ActiveIncidentCount(ctx context.Context, labID int64) (int, error) {
	var n int
	err := r.db.WithContext(ctx).Raw(
		`SELECT count(*) FROM lab_incidents WHERE lab_id = ? AND active`, labID,
	).Scan(&n).Error
	return n, err
}

const incidentCols = `id, title, break_script, reveal_md, rps, active`

func (r *CourseRepo) AdminIncidents(ctx context.Context, labID int64) ([]domain.Incident, error) {
	incidents := []domain.Incident{}
	err := r.db.WithContext(ctx).Raw(
		`SELECT `+incidentCols+` FROM lab_incidents
		  WHERE lab_id = ? ORDER BY active DESC, id`, labID,
	).Scan(&incidents).Error
	return incidents, err
}

func (r *CourseRepo) AdminIncident(ctx context.Context, incidentID int64) (*domain.Incident, error) {
	var inc domain.Incident
	res := r.db.WithContext(ctx).Raw(
		`SELECT `+incidentCols+` FROM lab_incidents WHERE id = ?`, incidentID,
	).Scan(&inc)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrIncidentNotFound
	}
	return &inc, nil
}

func (r *CourseRepo) CreateIncident(ctx context.Context, labID int64, in domain.IncidentInput) (*domain.Incident, error) {
	var id int64
	err := r.db.WithContext(ctx).Raw(
		`INSERT INTO lab_incidents (lab_id, title, break_script, reveal_md, rps, active)
		 VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		labID, in.Title, in.BreakScript, in.RevealMD, in.RPS, in.Active,
	).Scan(&id).Error
	if isMissingLab(err) {
		return nil, domain.ErrLabNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.AdminIncident(ctx, id)
}

func (r *CourseRepo) UpdateIncident(ctx context.Context, incidentID int64, in domain.IncidentInput) (*domain.Incident, error) {
	res := r.db.WithContext(ctx).Exec(
		`UPDATE lab_incidents SET title = ?, break_script = ?, reveal_md = ?, rps = ?, active = ?
		  WHERE id = ?`,
		in.Title, in.BreakScript, in.RevealMD, in.RPS, in.Active, incidentID,
	)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrIncidentNotFound
	}
	return r.AdminIncident(ctx, incidentID)
}

// DeleteIncident only succeeds for a scenario nobody has played. The foreign key
// from lab_sessions has no ON DELETE clause on purpose, so the database is the
// one that knows — asking it beforehand would be a second answer that can
// disagree with the first, and a race between the two.
func (r *CourseRepo) DeleteIncident(ctx context.Context, incidentID int64) error {
	res := r.db.WithContext(ctx).Exec(`DELETE FROM lab_incidents WHERE id = ?`, incidentID)
	if isStillReferenced(res.Error) {
		return domain.ErrIncidentPlayed
	}
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrIncidentNotFound
	}
	return nil
}
