package rest

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/labs/adapter/openrouter"
	"github.com/devforge/be/internal/labs/domain"
	"github.com/devforge/be/internal/labs/usecase/simgen"
)

// Simgen builds a playground scenario from a sentence. Sits beside Playground
// for the same reason Playground sits beside the labs: it runs the same engine,
// and it touches no session, no container and no mark.
//
// Deliberately playground-only. A graded lab reads its scenario from
// `labs.sim_scenario` server-side, and it stays that way: numbers a model
// invented are fine for a tool somebody is exploring with and are not fine for
// something that decides a score.
type Simgen struct{ uc *simgen.Simgen }

func NewSimgen(uc *simgen.Simgen) *Simgen { return &Simgen{uc: uc} }

type simgenTurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type simgenRequest struct {
	Prompt  string       `json:"prompt"`
	History []simgenTurn `json:"history"`
}

// Generate answers with a scenario the engine has already accepted and run every
// example of. It writes nothing: the result goes back to the browser, and what
// the user keeps is kept there.
func (s *Simgen) Generate(c *gin.Context) {
	var req simgenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "dữ liệu không hợp lệ")
		return
	}

	turns := make([]simgen.Turn, 0, len(req.History))
	for _, t := range req.History {
		turns = append(turns, simgen.Turn{Assistant: t.Role == "assistant", Text: t.Text})
	}

	res, err := s.uc.Generate(c.Request.Context(), userID(c), simgen.Input{
		Prompt:  req.Prompt,
		History: turns,
	})
	switch {
	case errors.Is(err, simgen.ErrDisabled), errors.Is(err, openrouter.ErrDisabled):
		// Not an error the person can act on and not a bug either: the feature
		// is switched off in this deployment. 503 rather than 500 so it reads as
		// "not available" in a log rather than as something crashing.
		abort(c, http.StatusServiceUnavailable,
			"tính năng nhờ AI dựng kịch bản chưa được bật trên máy chủ này")
	case errors.Is(err, domain.ErrAIQuota):
		abort(c, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, simgen.ErrEmptyPrompt), errors.Is(err, simgen.ErrPromptLong):
		abort(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, openrouter.ErrRefused):
		abort(c, http.StatusUnprocessableEntity,
			"model từ chối yêu cầu này — thử mô tả lại bằng lời khác")
	case errors.Is(err, simgen.ErrUnusable):
		// The model answered twice and neither answer ran. Say so plainly: the
		// alternative is handing over a scenario that looks right and breaks on
		// the first press of Run.
		abort(c, http.StatusUnprocessableEntity,
			"AI dựng ra kịch bản nhưng engine không chạy được, kể cả sau khi sửa lại. "+
				"Thử mô tả cụ thể hơn.")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, res)
	}
}
