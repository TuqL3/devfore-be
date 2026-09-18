package rest

import (
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

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
// (see deploy/nginx/devforge.conf). The meta refresh is the belt to that
// braces: a
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

// dayBoard parses the `:day` segment and reads that board. The preview page and
// its picture want exactly the same two steps, and a crawler walking the archive
// asks for the pair on every day it visits.
func (h *Handler) dayBoard(c *gin.Context) (*domain.DailyDrill, bool) {
	day, err := time.ParseInLocation(time.DateOnly, c.Param("day"), time.UTC)
	if err != nil {
		abort(c, http.StatusBadRequest, "ngày không hợp lệ")
		return nil, false
	}
	d, err := h.uc.Day(c.Request.Context(), day, time.Now())
	switch {
	case errors.Is(err, domain.ErrNoDailyDrill):
		// Same answer for a day nobody can have: tomorrow, or further back than
		// the archive reaches. A crawler that walks backwards forever needs a wall
		// to stop at, and 404 is that wall.
		abort(c, http.StatusNotFound, "chưa có ca trực nào được xuất bản")
		return nil, false
	case err != nil:
		serverError(c, err)
		return nil, false
	}
	return d, true
}

// DailyDrillPreview is the same trick as SharedDrillPreview, for the archive
// instead of one person's report.
//
// The back button on the War Room already read old boards, but only into React
// state — there was no URL to paste, so there was nothing for a crawler to fetch
// and nothing for a search engine to keep. `/war-room/day/<date>` is that URL,
// and this is what a crawler gets when it asks for one.
//
// The fault is not named here either, for the same reason as the shared report:
// the board describes a challenge that people are still meant to walk into cold.
// The lab title says which service broke, not what broke it.
func (h *Handler) DailyDrillPreview(c *gin.Context) {
	d, ok := h.dayBoard(c)
	if !ok {
		return
	}

	base := strings.TrimSuffix(h.frontendURL, "/")
	api := strings.TrimSuffix(h.publicURL, "/")
	title := "Ca trực " + d.Day
	desc := d.LabTitle + " — chưa ai cứu được ca này. Thử xem bạn có phải người đầu tiên."
	if len(d.Leaders) > 0 {
		top := d.Leaders[0]
		title = "Ca trực " + d.Day + ": " + top.Player + " nhanh nhất với " + mmss(top.DowntimeSeconds)
		desc = d.LabTitle + " — " + strconv.Itoa(len(d.Leaders)) +
			" người đã cứu được ca này. Thử xem bạn nhanh hơn không."
	}

	// Ten minutes for today's board, which is still collecting names, and a day
	// for one that has closed. A finished day never changes again, and the crawler
	// that walks the archive is exactly the client that would refetch it most.
	maxAge := "600"
	if d.Day != time.Now().UTC().Format(time.DateOnly) {
		maxAge = "86400"
	}
	c.Header("Cache-Control", "public, max-age="+maxAge)
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = previewTmpl.Execute(c.Writer, previewData{
		Title:       title,
		Description: desc,
		PageURL:     base + "/war-room/day/" + d.Day,
		ImageURL:    api + "/api/daily-drill/" + d.Day + "/og.png",
	})
}
