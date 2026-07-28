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
	cookies     CookieConfig
}

func NewHandler(auth *usecase.Auth, frontendURL, uploadDir, publicURL string, cookies CookieConfig) *Handler {
	return &Handler{
		auth:        auth,
		frontendURL: frontendURL,
		uploadDir:   uploadDir,
		publicURL:   publicURL,
		cookies:     cookies,
	}
}

func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.auth.Register(c.Request.Context(), req.toInput(), sessionMeta(c))
	if errors.Is(err, domain.ErrConflict) {
		abort(c, http.StatusConflict, "email hoặc username đã tồn tại")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	setCredentials(c, out.Credentials, h.cookies)
	c.JSON(http.StatusCreated, newUserResponse(out.User))
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if !bind(c, &req) {
		return
	}
	out, err := h.auth.Login(c.Request.Context(), req.toInput(), sessionMeta(c))
	switch {
	case errors.Is(err, domain.ErrCredentials):
		abort(c, http.StatusUnauthorized, "sai thông tin đăng nhập")
	case errors.Is(err, domain.ErrBanned):
		abort(c, http.StatusForbidden, "tài khoản đã bị khoá")
	case err != nil:
		serverError(c, err)
	default:
		setCredentials(c, out.Credentials, h.cookies)
		c.JSON(http.StatusOK, newUserResponse(out.User))
	}
}

// Refresh reads the session straight from the cookie — there is no request
// body to forge, and nothing for the page to hold on to.
func (h *Handler) Refresh(c *gin.Context) {
	cr, err := h.auth.Refresh(c.Request.Context(), SessionID(c), sessionMeta(c))
	if err != nil {
		// The cookies are dead weight now; leaving them would make the browser
		// retry a session the server has already refused.
		clearCredentials(c, h.cookies)
		abort(c, http.StatusUnauthorized, "phiên đăng nhập không hợp lệ")
		return
	}
	setCredentials(c, cr, h.cookies)
	c.Status(http.StatusNoContent)
}

// Logout ends this browser's session only.
func (h *Handler) Logout(c *gin.Context) {
	if err := h.auth.Logout(c.Request.Context(), SessionID(c)); err != nil {
		serverError(c, err)
		return
	}
	clearCredentials(c, h.cookies)
	c.Status(http.StatusNoContent)
}

// LogoutAll ends every session on the account, on every device. Requires a
// valid access token: this is exactly the button an attacker would want.
func (h *Handler) LogoutAll(c *gin.Context) {
	if err := h.auth.LogoutAll(c.Request.Context(), UserID(c)); err != nil {
		serverError(c, err)
		return
	}
	clearCredentials(c, h.cookies)
	c.Status(http.StatusNoContent)
}

// sessionMeta is what the request itself reveals about the device.
//
// Both values are for display only. The User-Agent is whatever the client
// chose to send, and ClientIP trusts X-Forwarded-For from the proxies listed in
// main.go — neither is evidence of anything, so neither is used for a decision.
func sessionMeta(c *gin.Context) usecase.SessionMeta {
	return usecase.SessionMeta{
		UserAgent: c.GetHeader("User-Agent"),
		IP:        c.ClientIP(),
	}
}

// Sessions lists the devices signed in to this account.
func (h *Handler) Sessions(c *gin.Context) {
	list, err := h.auth.Sessions(c.Request.Context(), UserID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, newSessionsResponse(list, SessionID(c)))
}

// RevokeSession signs out one device from the list.
func (h *Handler) RevokeSession(c *gin.Context) {
	id := c.Param("id")
	err := h.auth.RevokeSession(c.Request.Context(), UserID(c), id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		abort(c, http.StatusNotFound, "phiên không tồn tại")
	case err != nil:
		serverError(c, err)
	default:
		// Revoking the session you are currently on is allowed — it is just a
		// logout — but the cookies have to go with it.
		if id == SessionID(c) {
			clearCredentials(c, h.cookies)
		}
		c.Status(http.StatusNoContent)
	}
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
	err := h.auth.ChangePassword(c.Request.Context(), UserID(c), req.CurrentPassword, req.NewPassword, SessionID(c))
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
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthStateCookie, state, 300, "/", h.cookies.Domain, h.cookies.Secure, true)
	c.Redirect(http.StatusTemporaryRedirect, h.auth.OAuthURL(state))
}

func (h *Handler) GoogleCallback(c *gin.Context) {
	want, _ := c.Cookie(oauthStateCookie)
	if want == "" || c.Query("state") != want {
		h.redirectError(c, "state không hợp lệ")
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthStateCookie, "", -1, "/", h.cookies.Domain, h.cookies.Secure, true)

	out, err := h.auth.AuthenticateGoogle(c.Request.Context(), c.Query("code"), sessionMeta(c))
	switch {
	case errors.Is(err, domain.ErrBanned):
		h.redirectError(c, "tài khoản đã bị khoá")
		return
	case err != nil:
		slog.Error("google auth", "err", err)
		h.redirectError(c, "đăng nhập google thất bại")
		return
	}

	// The credentials go back as cookies, not in the URL. A fragment survives in
	// history, in the address bar, and in anything the page later logs.
	setCredentials(c, out.Credentials, h.cookies)
	c.Redirect(http.StatusTemporaryRedirect, h.frontendURL+"/auth/callback")
}

func (h *Handler) redirectError(c *gin.Context, msg string) {
	c.Redirect(http.StatusTemporaryRedirect, h.frontendURL+"/login?error="+url.QueryEscape(msg))
}
