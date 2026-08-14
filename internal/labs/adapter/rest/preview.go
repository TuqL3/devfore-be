package rest

import (
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/devforge/be/internal/labs/domain"
)

// The page a crawler reads when somebody pastes a shared link.
//
// The app is a single-page build: the HTML it ships is an empty shell, and the
// numbers only exist after JavaScript has run. Facebook, X, Zalo and Slack do
// not run it, so a link pasted anywhere renders as a blank rectangle with a
// domain under it. This endpoint is the fix — the same report, flattened into
// meta tags a crawler can read without executing anything.
//
// Reverse proxy sends crawler user agents here and everybody else to the app
// (see deploy/caddy/Caddyfile). The meta refresh is the belt to that braces: a
// human who reaches this URL directly lands on the real page rather than on a
// stub, and it costs one line.
var previewTmpl = template.Must(template.New("preview").Parse(`<!doctype html>
<html lang="vi">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<meta name="description" content="{{.Description}}">
<link rel="canonical" href="{{.PageURL}}">

<meta property="og:type" content="article">
<meta property="og:site_name" content="DevForge">
<meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Description}}">
<meta property="og:url" content="{{.PageURL}}">
<meta property="og:image" content="{{.ImageURL}}">
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">

<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="{{.Title}}">
<meta name="twitter:description" content="{{.Description}}">
<meta name="twitter:image" content="{{.ImageURL}}">

<meta http-equiv="refresh" content="0; url={{.PageURL}}">
</head>
<body>
<p>{{.Description}}</p>
<p><a href="{{.PageURL}}">Xem báo cáo</a></p>
</body>
</html>
`))

type previewData struct {
	Title       string
	Description string
	PageURL     string
	ImageURL    string
}

// SharedDrillPreview renders those tags. Public, and it says no more than the
// page it describes: a name, a fault, a time.
//
// The fault's title is deliberately left out of both the title and the
// description. On the page it sits behind a click that warns what the click
// costs; a preview card that spells it out in a timeline takes that choice away
// from every reader who scrolls past.
func (h *Handler) SharedDrillPreview(c *gin.Context) {
	d, err := h.uc.Shared(c.Request.Context(), c.Param("token"))
	if errors.Is(err, domain.ErrNotFound) {
		abort(c, http.StatusNotFound, "báo cáo này không còn được chia sẻ")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	base := strings.TrimSuffix(h.frontendURL, "/")
	api := strings.TrimSuffix(h.publicURL, "/")
	title := d.Player + " hết giờ với " + d.LabTitle
	desc := "Ca trực kết thúc mà dịch vụ vẫn chết. Thử đúng ca này trên DevForge War Room."
	if d.Recovered {
		title = d.Player + " cứu được sự cố trong " + mmss(d.DowntimeSeconds)
		desc = d.LabTitle + " — " + strconv.Itoa(d.RequestsFailed) +
			" request hỏng trước khi dịch vụ sống lại. Thử đúng ca này xem bạn nhanh hơn không."
	}

	// text/html, and the template escapes every field: the display name is
	// somebody else's text, and this is the one place on the platform where it is
	// rendered as markup rather than handed to a client as JSON.
	c.Header("Cache-Control", "public, max-age=600")
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = previewTmpl.Execute(c.Writer, previewData{
		Title:       title,
		Description: desc,
		PageURL:     base + "/r/" + d.Token,
		ImageURL:    api + "/api/shared-drills/" + d.Token + "/og.png",
	})
}
