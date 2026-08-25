package rest

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/inconsolata"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/devforge/be/internal/labs/domain"
)

// The card a chat app or a timeline shows in place of a shared link.
//
// 1200×630 is the size every platform crops from, and it is drawn here rather
// than screenshotted from the page: a headless browser to render one number is a
// second runtime to install, keep patched and pay for.
//
// The type is a bitmap font scaled up with nearest-neighbour, so the numbers come
// out blocky on purpose. Embedding a TTF would look smoother and would also mean
// carrying a font file and its licence in the repo; blocky reads as a terminal,
// which is what this product is. One dependency for the whole thing
// (golang.org/x/image), and no font file.
const (
	ogWidth  = 1200
	ogHeight = 630
)

var (
	ogBG      = color.RGBA{0x09, 0x09, 0x0b, 0xff}
	ogPanel   = color.RGBA{0x18, 0x18, 0x1b, 0xff}
	ogAccent  = color.RGBA{0xfb, 0xbf, 0x24, 0xff}
	ogSuccess = color.RGBA{0x4a, 0xde, 0x80, 0xff}
	ogDanger  = color.RGBA{0xf8, 0x71, 0x71, 0xff}
	ogFG      = color.RGBA{0xfa, 0xfa, 0xfa, 0xff}
	ogMuted   = color.RGBA{0xa1, 0xa1, 0xaa, 0xff}
)

// SharedDrillOG draws the preview image for one published drill.
//
// Public, like the page it illustrates: the crawler that fetches it has no
// account and no cookie. It carries exactly what the page carries — a name, a
// fault, a time — and 404s for a token that was taken down, so an unshared
// report stops having a picture too.
func (h *Handler) SharedDrillOG(c *gin.Context) {
	d, err := h.uc.Shared(c.Request.Context(), c.Param("token"))
	if errors.Is(err, domain.ErrNotFound) {
		abort(c, http.StatusNotFound, "báo cáo này không còn được chia sẻ")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	img := drawOGCard(d)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		serverError(c, err)
		return
	}
	// A published report never changes, so let the platforms keep the picture.
	// They cache aggressively either way; saying so out loud means a timeline
	// scrolling past a hundred of these does not fetch a hundred images.
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, "image/png", buf.Bytes())
}

func drawOGCard(d *domain.SharedDrill) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, ogWidth, ogHeight))
	fill(img, img.Bounds(), ogBG)

	// A band down the left edge in the colour of the outcome. The one thing
	// readable at the size a timeline actually shows this: recovered or not.
	outcome := ogDanger
	if d.Recovered {
		outcome = ogSuccess
	}
	fill(img, image.Rect(0, 0, 14, ogHeight), outcome)
	fill(img, image.Rect(80, 490, ogWidth-80, 491), ogPanel)

	text(img, 80, 100, 3, ogAccent, "WAR ROOM")
	text(img, 80, 150, 2, ogMuted, clip(ascii(d.LabTitle), 52))

	// The time is the headline. It is the number somebody pasted the link to
	// show, and the number a reader measures themselves against.
	//
	// Baselines are spaced by hand rather than by a layout pass: the card has
	// five lines that never change, and every one of them is placed clear of the
	// line above at its own scale. A first draft put the headline's baseline 120px
	// under the lab title, which at eleven times a 16px face meant the digits sat
	// on top of it.
	if d.Recovered {
		text(img, 80, 380, 11, ogFG, mmss(d.DowntimeSeconds))
		text(img, 80, 440, 3, ogMuted, ascii("da cuu duoc dich vu"))
		text(img, 80, 545, 3, outcome, clip(strings.ToUpper(ascii(d.Player)), 30))
		text(img, 80, 590, 2, ogMuted, strconv.Itoa(d.RequestsFailed)+" request hong")
	} else {
		text(img, 80, 370, 9, ogFG, "HET GIO")
		text(img, 80, 440, 3, ogMuted, ascii("dich vu van chet khi het gio"))
		text(img, 80, 545, 3, outcome, clip(strings.ToUpper(ascii(d.Player)), 30))
	}

	text(img, ogWidth-80-scaledWidth("DEVFORGE", 2), 570, 2, ogMuted, "DEVFORGE")
	return img
}

// DailyDrillOG draws the archive card: one closed day of the War Room.
//
// Same shape and same primitives as the report card, because the two land side
// by side in the same timeline and a reader should be able to tell at a glance
// that they come from the same place.
func (h *Handler) DailyDrillOG(c *gin.Context) {
	d, ok := h.dayBoard(c)
	if !ok {
		return
	}

	img := drawDayCard(d)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		serverError(c, err)
		return
	}
	// A closed day is frozen; today's board still gains names. The picture only
	// carries the fastest of them, so today's is the one worth re-fetching.
	maxAge := "600"
	if d.Day != time.Now().UTC().Format(time.DateOnly) {
		maxAge = "86400"
	}
	c.Header("Cache-Control", "public, max-age="+maxAge)
	c.Data(http.StatusOK, "image/png", buf.Bytes())
}

func drawDayCard(d *domain.DailyDrill) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, ogWidth, ogHeight))
	fill(img, img.Bounds(), ogBG)

	// Green once somebody got through it, red while the day still stands unbeaten
	// — the same colour language as the report card, read at the same glance.
	outcome := ogDanger
	if len(d.Leaders) > 0 {
		outcome = ogSuccess
	}
	fill(img, image.Rect(0, 0, 14, ogHeight), outcome)
	fill(img, image.Rect(80, 490, ogWidth-80, 491), ogPanel)

	text(img, 80, 100, 3, ogAccent, "WAR ROOM")
	text(img, 80, 150, 2, ogMuted, clip(ascii(d.LabTitle), 52))

	// The date is the headline: it is what the link promises and the one thing
	// that tells two archive cards apart in a feed. Baselines copied from the
	// report card rather than recomputed — the two must line up.
	if len(d.Leaders) > 0 {
		top := d.Leaders[0]
		text(img, 80, 360, 7, ogFG, d.Day)
		text(img, 80, 440, 3, ogMuted, ascii("nhanh nhat trong ngay"))
		text(img, 80, 545, 3, outcome, clip(strings.ToUpper(ascii(top.Player)), 30))
		text(img, 80, 590, 2, ogMuted, mmss(top.DowntimeSeconds)+" - "+
			strconv.Itoa(len(d.Leaders))+" nguoi cuu duoc")
	} else {
		text(img, 80, 360, 7, ogFG, d.Day)
		text(img, 80, 440, 3, ogMuted, ascii("chua ai cuu duoc ca nay"))
		text(img, 80, 545, 3, outcome, "CHUA CO AI")
	}

	text(img, ogWidth-80-scaledWidth("DEVFORGE", 2), 570, 2, ogMuted, "DEVFORGE")
	return img
}

func fill(dst *image.RGBA, r image.Rectangle, c color.Color) {
	draw.Draw(dst, r, &image.Uniform{C: c}, image.Point{}, draw.Src)
}

// text draws one line, magnified by an integer factor.
//
// Drawn at 1× into a scratch image and then blown up nearest-neighbour, because
// a bitmap face has exactly one size. An integer factor keeps every pixel square
// — a fractional one smears the edges and the blockiness stops reading as a
// choice.
func text(dst *image.RGBA, x, y, scale int, c color.Color, s string) {
	if s == "" {
		return
	}
	face := inconsolata.Bold8x16
	w := font.MeasureString(face, s).Ceil()
	h := face.Metrics().Height.Ceil()
	if w <= 0 || h <= 0 {
		return
	}

	small := image.NewRGBA(image.Rect(0, 0, w, h+4))
	(&font.Drawer{
		Dst:  small,
		Src:  &image.Uniform{C: c},
		Face: face,
		Dot:  fixed.P(0, h-2),
	}).DrawString(s)

	big := image.Rect(x, y-h*scale, x+w*scale, y+4*scale)
	xdraw.NearestNeighbor.Scale(dst, big, small, small.Bounds(), draw.Over, nil)
}

func scaledWidth(s string, scale int) int {
	return font.MeasureString(inconsolata.Bold8x16, s).Ceil() * scale
}

// mmss is the same m:ss the screens use, zero-padded so the headline keeps its
// width whether the recovery took six minutes or sixteen.
func mmss(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	m := seconds / 60
	return strconv.Itoa(m) + ":" + pad2(seconds%60)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// ascii strips the marks the bitmap face cannot draw.
//
// inconsolata covers Latin-1 and stops there, so a Vietnamese name or lab title
// comes out with holes in it — "Ca Trực Đầu Tiên" loses exactly the letters that
// carry the tone. Decomposing and dropping the combining marks leaves "Ca Truc
// Dau Tien": still recognisably itself, which is what a preview picture owes the
// reader. The page shows the real text; this is the card, not the record.
//
// Decomposition rather than a hand-written table of Vietnamese letters: the
// table was 60 lines, covered only uppercase, and would have been wrong the day
// somebody signed up with a name from another language. đ and Đ are the two that
// do not decompose, so they are named.
func ascii(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	folded, _, err := transform.String(t, strings.NewReplacer("đ", "d", "Đ", "D").Replace(s))
	if err != nil {
		folded = s
	}
	var b strings.Builder
	for _, r := range folded {
		if r < 0x80 {
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "?"
	}
	return out
}
