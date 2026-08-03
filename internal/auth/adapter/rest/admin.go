package rest

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/audit"
	"github.com/devforge/be/internal/auth/domain"
)

type managedUser struct {
	ID        int64      `json:"id"`
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	AvatarURL *string    `json:"avatar_url"`
	Status    string     `json:"status"`
	Roles     []string   `json:"roles"`
	CreatedAt time.Time  `json:"created_at"`
	BannedAt  *time.Time `json:"banned_at"`
	// Nil rather than "" when there is none, so the screen can tell "banned
	// without a reason given" from "reason is an empty string".
	BannedReason *string `json:"banned_reason"`
	BannedBy     *string `json:"banned_by"`
}

type userListResponse struct {
	Users []managedUser `json:"users"`
	Total int           `json:"total"`
	Limit int           `json:"limit"`
}

// AdminUsers lists accounts for the admin table. The filters are optional and
// unknown values simply match nothing rather than erroring: a client sending
// status=nonsense gets an empty list, which is what it asked for.
func (h *Handler) AdminUsers(c *gin.Context) {
	out, err := h.auth.ListUsers(c.Request.Context(), domain.UserFilter{
		Query:  c.Query("q"),
		Status: c.Query("status"),
		Role:   c.Query("role"),
	})
	if err != nil {
		serverError(c, err)
		return
	}
	users := make([]managedUser, len(out.Users))
	for i, u := range out.Users {
		users[i] = managedUser{
			ID:           u.ID,
			Username:     u.Username,
			Email:        u.Email,
			AvatarURL:    u.AvatarURL,
			Status:       string(u.Status),
			Roles:        u.Roles,
			CreatedAt:    u.CreatedAt,
			BannedAt:     u.BannedAt,
			BannedReason: u.BannedReason,
			BannedBy:     u.BannedBy,
		}
		if users[i].Roles == nil {
			users[i].Roles = []string{}
		}
	}
	c.JSON(http.StatusOK, userListResponse{Users: users, Total: out.Total, Limit: out.Limit})
}

type banRequest struct {
	Banned bool   `json:"banned"`
	Reason string `json:"reason" binding:"max=500"`
}

func (h *Handler) AdminSetBanned(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req banRequest
	if !bind(c, &req) {
		return
	}
	err := h.auth.SetBanned(c.Request.Context(), UserID(c), id, req.Banned, req.Reason)
	if err == nil {
		action := audit.ActionUserUnban
		if req.Banned {
			action = audit.ActionUserBan
		}
		h.audit.Record(c, audit.Entry{
			ActorID:    UserID(c),
			Action:     action,
			TargetType: audit.TargetUser,
			TargetID:   strconv.FormatInt(id, 10),
			Detail:     req.Reason,
		})
	}
	adminResult(c, err)
}

type roleRequest struct {
	Admin bool `json:"admin"`
}

func (h *Handler) AdminSetRole(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req roleRequest
	if !bind(c, &req) {
		return
	}
	err := h.auth.SetAdmin(c.Request.Context(), UserID(c), id, req.Admin)
	if err == nil {
		action := audit.ActionRoleRevoke
		if req.Admin {
			action = audit.ActionRoleGrant
		}
		h.audit.Record(c, audit.Entry{
			ActorID:    UserID(c),
			Action:     action,
			TargetType: audit.TargetUser,
			TargetID:   strconv.FormatInt(id, 10),
		})
	}
	adminResult(c, err)
}

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		abort(c, http.StatusBadRequest, "id không hợp lệ")
		return 0, false
	}
	return id, true
}

// adminResult maps the two refusals these endpoints share. Both are 409: the
// request was well formed and the caller was allowed to make it, the account it
// names is just not in a state the action applies to.
func adminResult(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "không tìm thấy tài khoản")
	case errors.Is(err, domain.ErrSelfTarget):
		abortCode(c, http.StatusConflict, "self_target",
			"không thao tác được trên chính tài khoản của bạn")
	case errors.Is(err, domain.ErrNotActive):
		abortCode(c, http.StatusConflict, "not_active",
			"chỉ khoá được tài khoản đang hoạt động")
	case err != nil:
		serverError(c, err)
	default:
		c.Status(http.StatusNoContent)
	}
}
