package i18n

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// The catalogue is keyed by the Vietnamese sentence, so the one failure mode is
// a message that gets reworded, added or moved without the translation
// following it. This reads the same literals the handlers pass and fails on any
// that the catalogue does not know — which is the only thing that stops the
// English UI from drifting back to Vietnamese one sentence at a time.
func TestCatalogueCoversSources(t *testing.T) {
	sites, err := ScanSources("..")
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) < 50 {
		t.Fatalf("scanned only %d messages — the scanner stopped seeing call sites", len(sites))
	}

	var missing []string
	seen := map[string]bool{}
	for _, s := range sites {
		if _, ok := en[s.Msg]; ok || seen[s.Msg] {
			continue
		}
		seen[s.Msg] = true
		missing = append(missing, s.Msg+"  ("+s.Where+")")
	}
	if len(missing) > 0 {
		t.Errorf("%d message(s) have no English translation:\n\t%s\n\nAdd them to `en` in i18n.go.",
			len(missing), strings.Join(missing, "\n\t"))
	}
}

// The other direction: an entry nobody sends any more is dead weight that reads
// as coverage. Not fatal — a message can legitimately be removed in the same
// commit that this test runs in — but it should not pile up unnoticed.
func TestCatalogueHasNoStrays(t *testing.T) {
	sites, err := ScanSources("..")
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	for _, s := range sites {
		used[s.Msg] = true
	}
	for k := range en {
		if !used[k] {
			t.Errorf("catalogue entry is never sent by any handler: %q", k)
		}
	}
}

func TestTranslate(t *testing.T) {
	if got := Translate(EN, "lỗi máy chủ"); got != "server error" {
		t.Errorf("EN: got %q", got)
	}
	// Vietnamese is the source language, so it is returned untouched even for a
	// message that has a translation.
	if got := Translate(VI, "lỗi máy chủ"); got != "lỗi máy chủ" {
		t.Errorf("VI: got %q", got)
	}
	// The fallback that keeps an unknown message readable instead of blank.
	if got := Translate(EN, "chưa dịch câu này"); got != "chưa dịch câu này" {
		t.Errorf("unknown: got %q", got)
	}
	if got := Translate(EN, ""); got != "" {
		t.Errorf("empty: got %q", got)
	}
}

func TestFromHeaderParsesBrowserFormats(t *testing.T) {
	cases := map[string]Lang{
		"en":                      EN,
		"en-GB":                   EN,
		"en-GB,en;q=0.9,vi;q=0.8": EN,
		" EN ":                    EN,
		"vi":                      VI,
		"vi-VN,vi;q=0.9,en;q=0.8": VI,
		"":                        VI,
		"fr-FR":                   VI,
		// A header that merely mentions English later must not win: the first
		// tag is the one the client actually prefers.
		"vi-VN,en;q=0.5": VI,
	}
	for header, want := range cases {
		if got := fromHeaderValue(header); got != want {
			t.Errorf("%q: got %q, want %q", header, got, want)
		}
	}
}

// FromHeader with a nil context is the WebSocket path, where there is no
// request to read: it must answer rather than panic.
func TestFromNilContext(t *testing.T) {
	if got := From(nil); got != VI {
		t.Errorf("nil context: got %q", got)
	}
}

// End to end over a real request: proves the two things unit-testing the parser
// cannot — that the header is read off the right place, and that `?lang=` beats
// it (the WebSocket path, where JavaScript cannot set a header).
func TestFromRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	call := func(target, header string) Lang {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, target, nil)
		if header != "" {
			c.Request.Header.Set("Accept-Language", header)
		}
		return From(c)
	}

	if got := call("/x", "en-GB,en;q=0.9"); got != EN {
		t.Errorf("header en: got %q", got)
	}
	if got := call("/x", ""); got != VI {
		t.Errorf("no header: got %q", got)
	}
	// The query wins even when the browser says otherwise, which is the whole
	// reason it exists.
	if got := call("/x?lang=en", "vi-VN"); got != EN {
		t.Errorf("query en over header vi: got %q", got)
	}
	if got := call("/x?lang=vi", "en-GB"); got != VI {
		t.Errorf("query vi over header en: got %q", got)
	}

	// And the message actually comes back translated.
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/x?lang=en", nil)
	if got := Msg(c, "lỗi máy chủ"); got != "server error" {
		t.Errorf("Msg: got %q", got)
	}
}
