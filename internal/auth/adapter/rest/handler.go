package rest

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/auth/domain"
	"github.com/devforge/be/internal/auth/usecase"
)

type Handler struct {
	auth        *usecase.Auth
	frontendURL string
	uploadDir   string
	publicURL   string
}

func NewHandler(auth *usecase.Auth, frontendURL, uploadDir, publicURL string) *Handler {
	return &Handler{
		auth:        auth,
		frontendURL: frontendURL,
		uploadDir:   uploadDir,
		publicURL:   publicURL,
	}
}

func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.auth.Register(c.Request.Context(), req.toInput())
	if errors.Is(err, domain.ErrConflict) {
		abort(c, http.StatusConflict, "email hoặc username đã tồn tại")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusCreated, newAuthResponse(out))
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.auth.Login(c.Request.Context(), req.toInput())
	switch {
	case errors.Is(err, domain.ErrCredentials):
		abort(c, http.StatusUnauthorized, "sai thông tin đăng nhập")
	case errors.Is(err, domain.ErrBanned):
		abort(c, http.StatusForbidden, "tài khoản đã bị khoá")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, newAuthResponse(out))
	}
}

func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if !bind(c, &req) {
		return
	}
	tp, err := h.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		abort(c, http.StatusUnauthorized, "refresh token không hợp lệ")
		return
	}
	c.JSON(http.StatusOK, newTokenResponse(tp))
}

func (h *Handler) Me(c *gin.Context) {
	u, err := h.auth.Me(c.Request.Context(), UserID(c))
	if err != nil {
		abort(c, http.StatusNotFound, "user không tồn tại")
		return
	}
	c.JSON(http.StatusOK, newUserResponse(u))
}

func (h *Handler) UpdateMe(c *gin.Context) {
	var req updateProfileRequest
	if !bind(c, &req) {
		return
	}
	u, err := h.auth.UpdateProfile(c.Request.Context(), UserID(c), usecase.UpdateProfileInput{
		Username:  req.Username,
		Email:     req.Email,
		AvatarURL: req.AvatarURL,
	})
	switch {
	case errors.Is(err, domain.ErrConflict):
		abort(c, http.StatusConflict, "username hoặc email đã được dùng")
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "user không tồn tại")
	case err != nil:
		serverError(c, err)
	default:
		c.JSON(http.StatusOK, newUserResponse(u))
	}
}

func (h *Handler) ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if !bind(c, &req) {
		return
	}
	err := h.auth.ChangePassword(c.Request.Context(), UserID(c), req.CurrentPassword, req.NewPassword)
	switch {
	case errors.Is(err, domain.ErrCredentials):
		abort(c, http.StatusForbidden, "mật khẩu hiện tại không đúng")
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "user không tồn tại")
	case err != nil:
		serverError(c, err)
	default:
		c.Status(http.StatusNoContent)
	}
}

func (h *Handler) DeleteMe(c *gin.Context) {
	var req deleteAccountRequest
	if !bind(c, &req) {
		return
	}
	err := h.auth.DeleteAccount(c.Request.Context(), UserID(c), req.Password)
	switch {
	case errors.Is(err, domain.ErrCredentials):
		abort(c, http.StatusForbidden, "mật khẩu không đúng")
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "user không tồn tại")
	case err != nil:
		serverError(c, err)
	default:
		c.Status(http.StatusNoContent)
	}
}

const oauthStateCookie = "df_oauth_state"

func (h *Handler) GoogleLogin(c *gin.Context) {
	if !h.auth.OAuthEnabled() {
		abort(c, http.StatusNotImplemented, "google login chưa cấu hình")
		return
	}
	state := newState()
	c.SetCookie(oauthStateCookie, state, 300, "/", "", false, true)
	c.Redirect(http.StatusTemporaryRedirect, h.auth.OAuthURL(state))
}

func (h *Handler) GoogleCallback(c *gin.Context) {
	want, _ := c.Cookie(oauthStateCookie)
	if want == "" || c.Query("state") != want {
		h.redirectError(c, "state không hợp lệ")
		return
	}
	c.SetCookie(oauthStateCookie, "", -1, "/", "", false, true)

	out, err := h.auth.AuthenticateGoogle(c.Request.Context(), c.Query("code"))
	switch {
	case errors.Is(err, domain.ErrBanned):
		h.redirectError(c, "tài khoản đã bị khoá")
		return
	case err != nil:
		slog.Error("google auth", "err", err)
		h.redirectError(c, "đăng nhập google thất bại")
		return
	}

	frag := url.Values{
		"access_token":  {out.Tokens.AccessToken},
		"refresh_token": {out.Tokens.RefreshToken},
	}
	c.Redirect(http.StatusTemporaryRedirect, h.frontendURL+"/auth/callback#"+frag.Encode())
}

func (h *Handler) redirectError(c *gin.Context, msg string) {
	c.Redirect(http.StatusTemporaryRedirect, h.frontendURL+"/login?error="+url.QueryEscape(msg))
}
