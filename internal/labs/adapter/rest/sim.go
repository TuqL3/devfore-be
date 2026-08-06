package rest

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase/sim"
)

// simRunRequest carries the YAML as text. The server parses it, not the client:
// a pipeline the browser decided was fine is a pipeline graded against a shape
// nobody on this side agreed to.
type simRunRequest struct {
	Pipeline string `json:"pipeline"`
}

type simRunResponse struct {
	RunIndex  int               `json:"run_index"`
	Pipeline  string            `json:"pipeline"`
	Result    *domain.RunResult `json:"result"`
	CreatedAt time.Time         `json:"created_at"`
}

// SimRun simulates the pipeline the student just wrote and stores the outcome.
// The whole timeline comes back in one response — there is nothing to stream,
// because the schedule is fully known the moment the engine returns.
func (h *Handler) SimRun(c *gin.Context) {
	var req simRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}

	res, err := h.uc.SimRun(c.Request.Context(), c.Param("id"), userID(c), req.Pipeline)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case errors.Is(err, domain.ErrNotRunning):
		abort(c, http.StatusConflict, "phiên lab đã kết thúc, hãy mở lại bài thực hành")
	case errors.Is(err, domain.ErrNotSimLab):
		abort(c, http.StatusBadRequest, "bài lab này không phải bài mô phỏng")
	case errors.Is(err, sim.ErrPipelineInvalid):
		// The only place a raw error string is shown to a student. Every sentence
		// Parse produces is written for them and names the job, step or key at
		// fault — passing it through is the point of wrapping them all in one error.
		abort(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrTooManyRuns):
		abort(c, http.StatusTooManyRequests, "phiên này đã chạy hết số lượt cho phép")
	case errors.Is(err, domain.ErrRunRaced):
		abort(c, http.StatusConflict, "một lượt chạy khác vừa được ghi, bấm Run lại")
	case err != nil:
		// Including ErrEmptyScenario and ErrInvalidScenario — an author's lab with
		// no catalogue, or with a number the engine refuses. Nothing the student can
		// do about either, so both read as a server fault rather than as a rejection
		// of the pipeline they just wrote.
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, res)
	}
}

// SimRuns is the history behind the run picker, oldest first.
func (h *Handler) SimRuns(c *gin.Context) {
	runs, err := h.uc.SimRuns(c.Request.Context(), c.Param("id"), userID(c))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên lab không tồn tại")
	case err != nil:
		serverError(c, err)
	default:
		out := make([]simRunResponse, 0, len(runs))
		for _, r := range runs {
			out = append(out, simRunResponse{
				RunIndex:  r.RunIndex,
				Pipeline:  r.Pipeline,
				Result:    r.Result,
				CreatedAt: r.CreatedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{"runs": out})
	}
}
