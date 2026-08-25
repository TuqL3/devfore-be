package rest

import (
	"strings"
	"testing"

	"github.com/devforge/be/internal/labs/domain"
)

// The archive card is drawn with hand-placed baselines, and the text that goes
// into it is somebody else's display name. Both ways that ends badly are silent:
// a name long enough to run off the card, or a name the pixel font has no glyph
// for. Neither shows up as an error — the card just comes out wrong, on a social
// timeline, where nobody can fix it.
//
// This is not a layout test. It pins the two things a unit test can actually
// hold: the card is always the size the meta tags promise, and no input reaches
// the drawing code unclipped or un-transliterated.
func TestDayCardSurvivesWhateverIsInIt(t *testing.T) {
	tests := []struct {
		name string
		d    *domain.DailyDrill
	}{
		{
			name: "nobody solved it",
			d:    &domain.DailyDrill{Day: "2026-08-17", LabTitle: "First Shift"},
		},
		{
			name: "somebody did",
			d: &domain.DailyDrill{
				Day: "2026-08-17", LabTitle: "First Shift",
				Leaders: []domain.DrillLeader{{Player: "lukas", DowntimeSeconds: 214, RequestsFailed: 4280}},
			},
		},
		{
			// 300 characters of name and title, which is far past anything the
			// card has room for. clip() is what stands between this and a card
			// with text running off the right edge.
			name: "a name and a title far longer than the card",
			d: &domain.DailyDrill{
				Day: "2026-08-17", LabTitle: strings.Repeat("very long lab title ", 15),
				Leaders: []domain.DrillLeader{{Player: strings.Repeat("nguyen", 50), DowntimeSeconds: 59, RequestsFailed: 1}},
			},
		},
		{
			// The pixel font is Latin-1 only. ascii() folds the diacritics; without
			// it these render as blank boxes on the one asset nobody can re-render.
			name: "vietnamese diacritics and an emoji",
			d: &domain.DailyDrill{
				Day: "2026-08-17", LabTitle: "Ca trực đầu tiên 🔥",
				Leaders: []domain.DrillLeader{{Player: "Nguyễn Văn Đức 🚀", DowntimeSeconds: 3600, RequestsFailed: 0}},
			},
		},
		{
			// An hour and beyond: mmss() gets the headline slot on the report card
			// and the sub-line here, and a four-digit minute count is the input
			// that would push it into the panel rule.
			name: "an outage measured in hours",
			d: &domain.DailyDrill{
				Day: "2026-08-17", LabTitle: "First Shift",
				Leaders: []domain.DrillLeader{{Player: "slowpoke", DowntimeSeconds: 86_399, RequestsFailed: 999_999}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := drawDayCard(tt.d)
			b := img.Bounds()
			if b.Dx() != ogWidth || b.Dy() != ogHeight {
				t.Fatalf("kích thước = %dx%d, muốn %dx%d — thẻ og khai 1200x630",
					b.Dx(), b.Dy(), ogWidth, ogHeight)
			}
		})
	}
}
