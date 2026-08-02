package rest

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/audit"
	"github.com/devforge/be/internal/auth/domain"
)

// challengeResponse is what a login answers with when the account has a second
// factor. Deliberately says nothing else: no user object, no roles, nothing that
// would make a half-finished login look finished to a client that forgot to read
// the flag.
type challengeResponse struct {
	MFARequired bool   `json:"mfa_required"`
	Challenge   string `json:"challenge"`
	ExpiresIn   int    `json:"expires_in"`
}

type mfaRequest struct {
	Challenge string `json:"challenge" binding:"required"`
	// Six digits from the authenticator, or one of the recovery codes.
	Code string `json:"code" binding:"required,max=64"`
}

// LoginMFA finishes a login parked by Login. Every failure answers 401 with the
// same message: which of "no such challenge", "wrong code" and "already spent"
// it was is exactly what an attacker would use to tell a valid challenge from a
// guess.
func (h *Handler) LoginMFA(c *gin.Context) {
	var req mfaRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.auth.CompleteLogin(c.Request.Context(), req.Challenge, req.Code, sessionMeta(c))
	switch {
	case errors.Is(err, domain.ErrRateLimited):
		abort(c, http.StatusTooManyRequests, "nhập sai quá nhiều lần, vui lòng đợi rồi thử lại")
	case errors.Is(err, domain.ErrBanned):
		abort(c, http.StatusForbidden, "tài khoản đã bị khoá")
	case err != nil && !errors.Is(err, domain.ErrInvalidToken) &&
		!errors.Is(err, domain.ErrInvalidCode) && !errors.Is(err, domain.ErrTOTPNotEnabled) &&
		!errors.Is(err, domain.ErrNotFound):
		serverError(c, err)
	case err != nil:
		abort(c, http.StatusUnauthorized, "mã xác thực không đúng hoặc đã hết hạn")
	default:
		setCredentials(c, out.Credentials, h.cookies)
		c.JSON(http.StatusOK, newUserResponse(out.User))
	}
}

type totpStatusResponse struct {
	Enabled      bool `json:"enabled"`
	RecoveryLeft int  `json:"recovery_left"`
}

func (h *Handler) TOTPStatus(c *gin.Context) {
	s, err := h.auth.TOTPStatus(c.Request.Context(), UserID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, totpStatusResponse{Enabled: s.Enabled, RecoveryLeft: s.RecoveryLeft})
}

type totpStartResponse struct {
	// The base32 secret, for typing in by hand when a camera is not an option.
	Secret string `json:"secret"`
	URI    string `json:"uri"`
	// PNG data URI. Inline rather than a URL of its own: it describes the same
	// secret as the field above it, so a second endpoint would be a second place
	// to get the authorisation right.
	QR string `json:"qr"`
}

// TOTPStart hands out a secret. This is the one response that ever contains it:
// once enrolment is confirmed the secret is never readable again, which is why
// the recovery codes exist.
func (h *Handler) TOTPStart(c *gin.Context) {
	e, err := h.auth.StartTOTP(c.Request.Context(), UserID(c))
	switch {
	case errors.Is(err, domain.ErrTOTPEnabled):
		abortCode(c, http.StatusConflict, "totp_enabled",
			"tài khoản đã bật xác thực hai lớp — tắt trước rồi bật lại")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, totpStartResponse{Secret: e.Secret, URI: e.URI, QR: e.QR})
	}
}

type codeRequest struct {
	Code string `json:"code" binding:"required,max=64"`
}

type recoveryResponse struct {
	// Shown once. There is no endpoint that returns these again, because they
	// are stored hashed and nothing can read them back.
	RecoveryCodes []string `json:"recovery_codes"`
}

func (h *Handler) TOTPConfirm(c *gin.Context) {
	var req codeRequest
	if !bind(c, &req) {
		return
	}
	codes, err := h.auth.ConfirmTOTP(c.Request.Context(), UserID(c), req.Code)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusConflict, "chưa bắt đầu cài đặt — bấm bật lại từ đầu")
	case errors.Is(err, domain.ErrTOTPEnabled):
		abortCode(c, http.StatusConflict, "totp_enabled", "tài khoản đã bật xác thực hai lớp")
	case errors.Is(err, domain.ErrInvalidCode):
		abort(c, http.StatusBadRequest, "mã không đúng — kiểm tra lại giờ trên điện thoại")
	case err != nil:
		serverError(c, err)
	default:
		h.audit.Record(c, audit.Entry{
			ActorID:    UserID(c),
			Action:     audit.ActionTOTPEnable,
			TargetType: audit.TargetUser,
			TargetID:   strconv.FormatInt(UserID(c), 10),
		})
		c.JSON(http.StatusOK, recoveryResponse{RecoveryCodes: codes})
	}
}

type disableRequest struct {
	Password string `json:"password"`
}

func (h *Handler) TOTPDisable(c *gin.Context) {
	var req disableRequest
	if !bind(c, &req) {
		return
	}
	err := h.auth.DisableTOTP(c.Request.Context(), UserID(c), req.Password)
	switch {
	case errors.Is(err, domain.ErrCredentials):
		abort(c, http.StatusUnauthorized, "mật khẩu không đúng")
	case errors.Is(err, domain.ErrTOTPNotEnabled):
		abort(c, http.StatusConflict, "tài khoản chưa bật xác thực hai lớp")
	case err != nil:
		serverError(c, err)
	default:
		h.audit.Record(c, audit.Entry{
			ActorID:    UserID(c),
			Action:     audit.ActionTOTPDisable,
			TargetType: audit.TargetUser,
			TargetID:   strconv.FormatInt(UserID(c), 10),
		})
		c.Status(http.StatusNoContent)
	}
}
