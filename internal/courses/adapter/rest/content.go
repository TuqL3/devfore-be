package rest

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/courses/domain"
	"github.com/devforge/be/internal/upload"
)

// adminLab is the lab as its author sees it: the public labResponse leaves out
// the image and the ordering, which are authoring concerns.
type adminLab struct {
	ID              int64  `json:"id"`
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	DescriptionMD   string `json:"description_md"`
	DurationMinutes int    `json:"duration_minutes"`
	OrderIdx        int    `json:"order_idx"`
	LabImageID      *int64 `json:"lab_image_id"`
	// null on a container lab. Sent back as JSON rather than as a string so the
	// form can pretty-print it without parsing a string that happens to be JSON.
	SimScenario json.RawMessage `json:"sim_scenario"`
	// Shell that stands up the service a War Room scenario breaks. Admin-only,
	// like lab_image_id and for a stronger reason: it describes the surface the
	// fault is hidden in.
	IncidentSetup string `json:"incident_setup"`
	TaskCount     int    `json:"task_count"`
	Points        int    `json:"points"`
	// Active scenarios. Non-zero is the only thing that marks this lab a drill,
	// so the list uses it to badge one.
	IncidentCount int `json:"incident_count"`
}

func newAdminLab(l domain.Lab) adminLab {
	return adminLab{
		ID:              l.ID,
		Slug:            l.Slug,
		Title:           l.Title,
		DescriptionMD:   l.DescriptionMD,
		DurationMinutes: l.DurationMinutes,
		OrderIdx:        l.OrderIdx,
		LabImageID:      l.LabImageID,
		SimScenario:     rawJSON(l.SimScenario),
		IncidentSetup:   l.IncidentSetup,
		TaskCount:       l.TaskCount,
		Points:          l.Points,
		IncidentCount:   l.IncidentCount,
	}
}

// adminTask carries check_script. This is the only response in the codebase that
// does, and it is reachable only behind the admin role — the student-facing
// presenter has a test asserting the field never appears there.
type adminOption struct {
	Text    string `json:"text"`
	Correct bool   `json:"correct"`
}

type adminTask struct {
	ID               int64         `json:"id"`
	Title            string        `json:"title"`
	Hint             string        `json:"hint"`
	Points           int           `json:"points"`
	OrderIdx         int           `json:"order_idx"`
	Kind             string        `json:"kind"`
	CheckScript      string        `json:"check_script"`
	Options          []adminOption `json:"options"`
	ExpectedCommands string        `json:"expected_commands"`
	// The pass condition of a sim task. Same class of secret as check_script, and
	// it travels the same single route: this response, behind the admin role.
	SimGoal json.RawMessage `json:"sim_goal"`
}

func newAdminTask(t domain.AdminTask) adminTask {
	options := make([]adminOption, len(t.Options))
	for i, o := range t.Options {
		options[i] = adminOption{Text: o.Text, Correct: o.Correct}
	}
	return adminTask{
		ID:               t.ID,
		Title:            t.Title,
		Hint:             t.Hint,
		Points:           t.Points,
		OrderIdx:         t.OrderIdx,
		Kind:             t.Kind,
		CheckScript:      t.CheckScript,
		Options:          options,
		ExpectedCommands: t.ExpectedCommands,
		SimGoal:          rawJSON(t.SimGoal),
	}
}

// rawJSON turns a column's bytes into something json.Marshal will emit as JSON.
// The nil check is not decoration: an empty, non-nil json.RawMessage fails to
// marshal, which would take down the whole response over a column that is simply
// unset.
func rawJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}

type labImageResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Tag         string `json:"tag"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
}

type labInput struct {
	Slug            string          `json:"slug"`
	Title           string          `json:"title"`
	DescriptionMD   string          `json:"description_md"`
	DurationMinutes int             `json:"duration_minutes"`
	LabImageID      *int64          `json:"lab_image_id"`
	SimScenario     json.RawMessage `json:"sim_scenario"`
	IncidentSetup   string          `json:"incident_setup"`
	OrderIdx        int             `json:"order_idx"`
}

func (in labInput) toDomain() domain.LabInput {
	return domain.LabInput{
		Slug:            in.Slug,
		Title:           in.Title,
		DescriptionMD:   in.DescriptionMD,
		DurationMinutes: in.DurationMinutes,
		LabImageID:      in.LabImageID,
		SimScenario:     in.SimScenario,
		IncidentSetup:   in.IncidentSetup,
		OrderIdx:        in.OrderIdx,
	}
}

type taskInput struct {
	Title            string          `json:"title"`
	Hint             string          `json:"hint"`
	Kind             string          `json:"kind"`
	CheckScript      string          `json:"check_script"`
	Options          []adminOption   `json:"options"`
	ExpectedCommands string          `json:"expected_commands"`
	SimGoal          json.RawMessage `json:"sim_goal"`
	Points           int             `json:"points"`
	OrderIdx         int             `json:"order_idx"`
}

func (in taskInput) toDomain() domain.TaskInput {
	options := make([]domain.Option, len(in.Options))
	for i, o := range in.Options {
		options[i] = domain.Option{Text: o.Text, Correct: o.Correct}
	}
	return domain.TaskInput{
		Title:            in.Title,
		Hint:             in.Hint,
		Kind:             in.Kind,
		CheckScript:      in.CheckScript,
		Options:          options,
		ExpectedCommands: in.ExpectedCommands,
		SimGoal:          in.SimGoal,
		Points:           in.Points,
		OrderIdx:         in.OrderIdx,
	}
}

func (h *Handler) AdminLabs(c *gin.Context) {
	id, ok := courseID(c)
	if !ok {
		return
	}
	labs, err := h.uc.AdminLabs(c.Request.Context(), id)
	if writeCourseError(c, err) {
		return
	}
	out := make([]adminLab, len(labs))
	for i, l := range labs {
		out[i] = newAdminLab(l)
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) AdminCreateLab(c *gin.Context) {
	id, ok := courseID(c)
	if !ok {
		return
	}
	var in labInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	lab, err := h.uc.CreateLab(c.Request.Context(), id, in.toDomain())
	if writeCourseError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, newAdminLab(*lab))
}

func (h *Handler) AdminUpdateLab(c *gin.Context) {
	id, ok := pathID(c, "labID")
	if !ok {
		return
	}
	var in labInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	lab, err := h.uc.UpdateLab(c.Request.Context(), id, in.toDomain())
	if writeCourseError(c, err) {
		return
	}
	c.JSON(http.StatusOK, newAdminLab(*lab))
}

func (h *Handler) AdminDeleteLab(c *gin.Context) {
	id, ok := pathID(c, "labID")
	if !ok {
		return
	}
	if writeCourseError(c, h.uc.DeleteLab(c.Request.Context(), id)) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) AdminTasks(c *gin.Context) {
	id, ok := pathID(c, "labID")
	if !ok {
		return
	}
	tasks, err := h.uc.AdminTasks(c.Request.Context(), id)
	if writeCourseError(c, err) {
		return
	}
	out := make([]adminTask, len(tasks))
	for i, t := range tasks {
		out[i] = newAdminTask(t)
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) AdminCreateTask(c *gin.Context) {
	id, ok := pathID(c, "labID")
	if !ok {
		return
	}
	var in taskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	task, err := h.uc.CreateTask(c.Request.Context(), id, in.toDomain())
	if writeCourseError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, newAdminTask(*task))
}

func (h *Handler) AdminUpdateTask(c *gin.Context) {
	id, ok := pathID(c, "taskID")
	if !ok {
		return
	}
	var in taskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	task, err := h.uc.UpdateTask(c.Request.Context(), id, in.toDomain())
	if writeCourseError(c, err) {
		return
	}
	c.JSON(http.StatusOK, newAdminTask(*task))
}

func (h *Handler) AdminDeleteTask(c *gin.Context) {
	id, ok := pathID(c, "taskID")
	if !ok {
		return
	}
	if writeCourseError(c, h.uc.DeleteTask(c.Request.Context(), id)) {
		return
	}
	c.Status(http.StatusNoContent)
}

// The revision notes behind a course's "Ôn tập" tab. Addressed by course id on
// the way in and by their own id afterwards, the same way labs are.
func (h *Handler) AdminReviews(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	reviews, err := h.uc.AdminReviews(c.Request.Context(), id)
	if writeCourseError(c, err) {
		return
	}
	c.JSON(http.StatusOK, newReviewList(reviews))
}

func (h *Handler) AdminCreateReview(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	var in reviewInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	review, err := h.uc.CreateReview(c.Request.Context(), id, in.toDomain())
	if writeCourseError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, newReview(*review))
}

func (h *Handler) AdminUpdateReview(c *gin.Context) {
	id, ok := pathID(c, "reviewID")
	if !ok {
		return
	}
	var in reviewInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	review, err := h.uc.UpdateReview(c.Request.Context(), id, in.toDomain())
	if writeCourseError(c, err) {
		return
	}
	c.JSON(http.StatusOK, newReview(*review))
}

func (h *Handler) AdminDeleteReview(c *gin.Context) {
	id, ok := pathID(c, "reviewID")
	if !ok {
		return
	}
	if writeCourseError(c, h.uc.DeleteReview(c.Request.Context(), id)) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) AdminLabImages(c *gin.Context) {
	images, err := h.uc.LabImages(c.Request.Context())
	if writeCourseError(c, err) {
		return
	}
	out := make([]labImageResponse, len(images))
	for i, im := range images {
		out[i] = labImageResponse{
			ID:          im.ID,
			Name:        im.Name,
			Tag:         im.Tag,
			Description: im.Description,
			Active:      im.Active,
		}
	}
	c.JSON(http.StatusOK, out)
}

// AdminUploadImage stores a cover image and answers with its URL, which the
// form then saves as the course's image_url. Upload and save are separate steps
// because a course being created has no id yet to attach a file to.
func (h *Handler) AdminUploadImage(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, upload.MaxImageBytes+1024)
	fh, err := c.FormFile("file")
	if err != nil {
		if upload.ExceedsLimit(err) {
			abort(c, http.StatusRequestEntityTooLarge, "ảnh vượt quá 2MB")
			return
		}
		abort(c, http.StatusBadRequest, "thiếu file ảnh")
		return
	}

	url, err := upload.Saver{
		Dir:       h.uploadDir,
		PublicURL: h.publicURL,
		// Kept apart from avatars so a directory listing says what a file is.
		Subdir: "courses",
	}.SaveImage(fh)
	switch {
	case errors.Is(err, upload.ErrNotConfigured):
		abort(c, http.StatusNotImplemented, "chưa cấu hình nơi lưu ảnh")
	case errors.Is(err, upload.ErrTooLarge):
		abort(c, http.StatusRequestEntityTooLarge, "ảnh vượt quá 2MB")
	case errors.Is(err, upload.ErrUnsupportedType):
		abort(c, http.StatusUnsupportedMediaType, "chỉ nhận ảnh png, jpg, gif hoặc webp")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusCreated, gin.H{"url": url})
	}
}

func pathID(c *gin.Context, param string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(param), 10, 64)
	if err != nil || id <= 0 {
		abort(c, http.StatusBadRequest, "id không hợp lệ")
		return 0, false
	}
	return id, true
}

// ── War Room scenarios ─────────────────────────────────────────────────────

// adminIncident carries break_script. Along with adminTask.check_script and
// adminTask.sim_goal it is one of the three responses in the codebase that
// serialises an answer key, and it travels the same single route: behind the
// admin role, never on anything a student can reach.
type adminIncident struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	BreakScript string `json:"break_script"`
	RevealMD    string `json:"reveal_md"`
	RPS         int    `json:"rps"`
	Active      bool   `json:"active"`
}

func newAdminIncident(i domain.Incident) adminIncident {
	return adminIncident{
		ID:          i.ID,
		Title:       i.Title,
		BreakScript: i.BreakScript,
		RevealMD:    i.RevealMD,
		RPS:         i.RPS,
		Active:      i.Active,
	}
}

type incidentInput struct {
	Title       string `json:"title"`
	BreakScript string `json:"break_script"`
	RevealMD    string `json:"reveal_md"`
	RPS         int    `json:"rps"`
	Active      bool   `json:"active"`
}

func (in incidentInput) toDomain() domain.IncidentInput {
	return domain.IncidentInput{
		Title:       in.Title,
		BreakScript: in.BreakScript,
		RevealMD:    in.RevealMD,
		RPS:         in.RPS,
		Active:      in.Active,
	}
}

type adminDrill struct {
	ID              int64  `json:"id"`
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	DurationMinutes int    `json:"duration_minutes"`
	LabImageID      *int64 `json:"lab_image_id"`
	IncidentSetup   string `json:"incident_setup"`
	Status          string `json:"status"`
	// null for a challenge that belongs to no course, which is every one made in
	// War Room since 000028.
	CourseID      *int64 `json:"course_id"`
	CourseTitle   string `json:"course_title"`
	IncidentCount int    `json:"incident_count"`
	ScenarioCount int    `json:"scenario_count"`
}

func (h *Handler) AdminDrills(c *gin.Context) {
	drills, err := h.uc.AdminDrills(c.Request.Context())
	if writeCourseError(c, err) {
		return
	}
	out := make([]adminDrill, len(drills))
	for i, d := range drills {
		out[i] = adminDrill{
			ID:              d.ID,
			Slug:            d.Slug,
			Title:           d.Title,
			DurationMinutes: d.DurationMinutes,
			LabImageID:      d.LabImageID,
			IncidentSetup:   d.IncidentSetup,
			Status:          d.Status,
			CourseID:        d.CourseID,
			CourseTitle:     d.CourseTitle,
			IncidentCount:   d.IncidentCount,
			ScenarioCount:   d.ScenarioCount,
		}
	}
	c.JSON(http.StatusOK, out)
}

type drillStatusInput struct {
	Status string `json:"status"`
}

// AdminCreateDrill makes a War Room challenge. Its own route rather than the
// lab one, because a challenge has no course to be nested under — that is the
// whole difference between the two.
func (h *Handler) AdminCreateDrill(c *gin.Context) {
	var in labInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	lab, err := h.uc.CreateDrill(c.Request.Context(), in.toDomain())
	if writeCourseError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, newAdminLab(*lab))
}

func (h *Handler) AdminSetDrillStatus(c *gin.Context) {
	id, ok := pathID(c, "labID")
	if !ok {
		return
	}
	var in drillStatusInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	if writeCourseError(c, h.uc.SetDrillStatus(c.Request.Context(), id, in.Status)) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) AdminIncidents(c *gin.Context) {
	id, ok := pathID(c, "labID")
	if !ok {
		return
	}
	incidents, err := h.uc.AdminIncidents(c.Request.Context(), id)
	if writeCourseError(c, err) {
		return
	}
	out := make([]adminIncident, len(incidents))
	for i, inc := range incidents {
		out[i] = newAdminIncident(inc)
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) AdminCreateIncident(c *gin.Context) {
	id, ok := pathID(c, "labID")
	if !ok {
		return
	}
	var in incidentInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	inc, err := h.uc.CreateIncident(c.Request.Context(), id, in.toDomain())
	if writeCourseError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, newAdminIncident(*inc))
}

func (h *Handler) AdminUpdateIncident(c *gin.Context) {
	id, ok := pathID(c, "incidentID")
	if !ok {
		return
	}
	var in incidentInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	inc, err := h.uc.UpdateIncident(c.Request.Context(), id, in.toDomain())
	if writeCourseError(c, err) {
		return
	}
	c.JSON(http.StatusOK, newAdminIncident(*inc))
}

func (h *Handler) AdminDeleteIncident(c *gin.Context) {
	id, ok := pathID(c, "incidentID")
	if !ok {
		return
	}
	if writeCourseError(c, h.uc.DeleteIncident(c.Request.Context(), id)) {
		return
	}
	c.Status(http.StatusNoContent)
}
