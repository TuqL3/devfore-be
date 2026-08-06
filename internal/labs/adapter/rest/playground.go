package rest

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase"
	"github.com/devforge/be/internal/labs/usecase/sim"
)

// Playground is the simulator with no lab behind it: no session, no container,
// no mark, and — since the scenarios moved into the frontend's own source — no
// database either. One endpoint, and it only computes.
type Playground struct{ uc *usecase.Playground }

func NewPlayground(uc *usecase.Playground) *Playground { return &Playground{uc: uc} }

type previewRequest struct {
	// The scenario travels with the request because it lives in the client's
	// bundle, not in a table. See the usecase for why that is safe here and
	// deliberately not how the graded path works.
	Scenario json.RawMessage `json:"scenario"`
	Pipeline string          `json:"pipeline"`
	Run      int             `json:"run"`
	Warm     []string        `json:"warm"`
}

// Preview runs a pipeline and answers with the schedule. It reads nothing and
// writes nothing.
func (p *Playground) Preview(c *gin.Context) {
	var req previewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}
	var sc domain.Scenario
	if err := json.Unmarshal(req.Scenario, &sc); err != nil {
		abort(c, http.StatusUnprocessableEntity, "kịch bản không phải JSON hợp lệ")
		return
	}

	res, err := p.uc.Preview(c.Request.Context(), userID(c), &sc, usecase.PreviewInput{
		Pipeline: req.Pipeline,
		Run:      req.Run,
		Warm:     req.Warm,
	})
	switch {
	case errors.Is(err, domain.ErrInvalidScenario):
		abort(c, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, sim.ErrPipelineInvalid):
		// The one place a raw error reaches a person: every sentence Parse writes
		// names the job, step or key at fault, and that is the point.
		abort(c, http.StatusBadRequest, err.Error())
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, res)
	}
}
