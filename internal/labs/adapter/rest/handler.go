package rest

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/audit"
	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase"
)

type Handler struct {
	uc    *usecase.Labs
	audit *audit.Recorder
}

func NewHandler(uc *usecase.Labs) *Handler { return &Handler{uc: uc} }

// SetAudit hands the handler the recorder for the one endpoint that ends
// somebody else's work. Set after construction so labs and audit stay
// independent packages rather than a pair with a wiring order.
func (h *Handler) SetAudit(a *audit.Recorder) { h.audit = a }

type sessionResponse struct {
	ID    string `json:"id"`
	LabID int64  `json:"lab_id"`
	// The slugs the lab screen is addressed by. Sent so a client that is blocked
	// by this session can link straight to it instead of asking the student to
	// find a course they may not remember picking.
	LabSlug      string    `json:"lab_slug"`
	CourseSlug   string    `json:"course_slug"`
	Status       string    `json:"status"`
	StartedAt    time.Time `json:"started_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	SecondsLeft  int       `json:"seconds_left"`
	TerminalPath string    `json:"terminal_path"`
	// Which of this lab's tasks the student has already passed. Sent with the
	// session so a reload restores the ticks without a second round trip.
	PassedTaskIDs []int64 `json:"passed_task_ids"`
	// Null unless this session drew a fault. Carries the assumed request rate and
	// nothing else: everything else about the scenario — its name, what it broke,
	// how it is found — is what the student is in there working out.
	Incident *sessionIncident `json:"incident"`
}

type sessionIncident struct {
	RPS int `json:"rps"`
}

// respondSession answers with the session and the progress that belongs to it.
// A failure to read the progress does not fail the response: the terminal is
// what the student came for, and a missing tick costs them one press of a check
// that awards nothing the second time.
func (h *Handler) respondSession(c *gin.Context, code int, s *domain.Session) {
	res := newSessionResponse(s)
	ids, err := h.uc.PassedTaskIDs(c.Request.Context(), s.ID)
	if err != nil {
		slog.Error("read lab progress", "session", s.ID, "err", err)
	} else {
		res.PassedTaskIDs = ids
	}
	c.JSON(code, res)
}

func newSessionResponse(s *domain.Session) sessionResponse {
	var inc *sessionIncident
	if s.IncidentID != nil {
		inc = &sessionIncident{RPS: s.IncidentRPS}
	}
	return sessionResponse{
		Incident:    inc,
		ID:          s.ID,
		LabID:       s.LabID,
		LabSlug:     s.LabSlug,
		CourseSlug:  s.CourseSlug,
		Status:      string(s.Status),
		StartedAt:   s.StartedAt,
		ExpiresAt:   s.ExpiresAt,
		SecondsLeft: int(usecase.Remaining(s).Seconds()),
		// Never null in JSON: the client reads it as a list on every load.
		PassedTaskIDs: []int64{},
		// Handed to the client rather than built there, so the route can move
		// without a second repository needing to be edited in step.
		TerminalPath: "/ws/terminal/" + s.ID,
	}
}

func (h *Handler) Start(c *gin.Context) {
	out, err := h.uc.Start(c.Request.Context(), userID(c), c.Param("slug"))
	switch {
	case errors.Is(err, domain.ErrLabNotFound):
		abort(c, http.StatusNotFound, "bài lab không tồn tại")
	case errors.Is(err, domain.ErrAlreadyRunning):
		abort(c, http.StatusConflict, "bạn đang có một phiên lab chạy dở, hãy đóng nó trước")
	case errors.Is(err, domain.ErrNotEnrolled):
		abort(c, http.StatusForbidden, "bạn cần đăng ký khoá học này trước khi làm lab")
	case err != nil:
		serverError(c, err)
	default:
		h.respondSession(c, http.StatusCreated, out.Session)
	}
}

// Current lets a client that reloaded the page find its way back to the session
// it already owns, instead of pressing Start and being told it has one.
func (h *Handler) Current(c *gin.Context) {
	s, err := h.uc.Running(c.Request.Context(), userID(c))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		c.JSON(http.StatusOK, gin.H{"session": nil})
	case err != nil:
		serverError(c, err)
	default:
		res := newSessionResponse(s)
		if ids, err := h.uc.PassedTaskIDs(c.Request.Context(), s.ID); err != nil {
			slog.Error("read lab progress", "session", s.ID, "err", err)
		} else {
			res.PassedTaskIDs = ids
		}
		c.JSON(http.StatusOK, gin.H{"session": res})
	}
}

func (h *Handler) Session(c *gin.Context) {
	s, err := h.uc.Owned(c.Request.Context(), c.Param("id"), userID(c))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case err != nil:
		serverError(c, err)
	default:
		h.respondSession(c, http.StatusOK, s)
	}
}

func (h *Handler) Stop(c *gin.Context) {
	err := h.uc.Stop(c.Request.Context(), c.Param("id"), userID(c))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case errors.Is(err, domain.ErrNotRunning):
		// Already gone is the outcome the caller wanted.
		c.Status(http.StatusNoContent)
	case err != nil:
		serverError(c, err)
	default:
		c.Status(http.StatusNoContent)
	}
}

// checkResponse deliberately carries no output from the script. What a check
// prints is written by the course author and routinely names the exact path or
// content being looked for, which is the answer. Passing or not is the whole
// result a student is owed; the hint tab is where help belongs.
type checkResponse struct {
	Passed        bool `json:"passed"`
	PointsAwarded int  `json:"points_awarded"`
	LabCompleted  bool `json:"lab_completed"`
}

func (h *Handler) Check(c *gin.Context) {
	taskID, err := strconv.ParseInt(c.Param("taskID"), 10, 64)
	if err != nil {
		abort(c, http.StatusBadRequest, "task id không hợp lệ")
		return
	}

	// Body is optional: a script task has nothing to submit, and an old client
	// that sends none must not start failing.
	var body struct {
		Selected []int `json:"selected"`
	}
	_ = c.ShouldBindJSON(&body)

	g, err := h.uc.Check(c.Request.Context(), c.Param("id"), userID(c), taskID, body.Selected)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case errors.Is(err, domain.ErrNotRunning):
		abort(c, http.StatusConflict, "phiên lab đã kết thúc, hãy mở lại bài thực hành")
	case errors.Is(err, domain.ErrTaskNotFound), errors.Is(err, domain.ErrTaskNotInLab):
		// Same answer for both: which task ids exist is not something to map out
		// for a caller sending ids that are not theirs.
		abort(c, http.StatusNotFound, "nhiệm vụ không tồn tại")
	case errors.Is(err, domain.ErrCheckTimeout):
		abort(c, http.StatusGatewayTimeout, "bài kiểm tra chạy quá lâu, thử lại")
	case errors.Is(err, domain.ErrNoSimRun):
		abort(c, http.StatusConflict, "hãy chạy pipeline một lượt trước khi nộp")
	case errors.Is(err, domain.ErrEmptyGoal):
		// An author saved a sim task with no pass condition. Refused rather than
		// passed, and said out loud: silently marking it correct is the failure
		// nobody would report.
		abort(c, http.StatusConflict, "nhiệm vụ này chưa có điều kiện chấm")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, checkResponse{
			Passed:        g.Passed,
			PointsAwarded: g.PointsAwarded,
			LabCompleted:  g.LabCompleted,
		})
	}
}

// tryScriptRequest is an author testing a script before saving it, which is why
// the script arrives in the body rather than being read from the task row.
type tryScriptRequest struct {
	LabID int64 `json:"lab_id"`
	// Optional command that performs the task, so the author can see the script
	// pass as well as fail.
	Setup  string `json:"setup"`
	Script string `json:"script"`
}

type tryScriptResponse struct {
	ExitCode int    `json:"exit_code"`
	Passed   bool   `json:"passed"`
	Output   string `json:"output"`
	// Only meaningful when the author supplied a setup command.
	SetupExitCode int    `json:"setup_exit_code"`
	SetupOutput   string `json:"setup_output"`
	SetupFailed   bool   `json:"setup_failed"`
}

// TryScript is admin-only. It runs arbitrary shell in a throwaway sandbox — the
// same one a student gets, so no network, no root, read-only root filesystem —
// but it is still arbitrary shell, and the role check is what keeps it that way.
func (h *Handler) TryScript(c *gin.Context) {
	var req tryScriptRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.LabID <= 0 {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}

	res, err := h.uc.TryScript(c.Request.Context(), req.LabID, req.Setup, req.Script)
	switch {
	case errors.Is(err, domain.ErrEmptyScript):
		abort(c, http.StatusBadRequest, "chưa có script để chạy thử")
	case errors.Is(err, domain.ErrNoImage):
		abort(c, http.StatusBadRequest, "lab chưa gán image — chọn image rồi thử lại")
	case errors.Is(err, domain.ErrCheckTimeout):
		abort(c, http.StatusGatewayTimeout, "script chạy quá 10 giây")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, tryScriptResponse{
			ExitCode:      res.ExitCode,
			Passed:        !res.SetupFailed && res.ExitCode == 0,
			Output:        res.Output,
			SetupExitCode: res.SetupExitCode,
			SetupOutput:   res.SetupOutput,
			SetupFailed:   res.SetupFailed,
		})
	}
}

func abort(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}

func serverError(c *gin.Context, err error) {
	slog.Error("labs handler error", "path", c.FullPath(), "err", err)
	abort(c, http.StatusInternalServerError, "lỗi máy chủ")
}

func userID(c *gin.Context) int64 {
	v, _ := c.Get("user_id")
	id, _ := v.(int64)
	return id
}
