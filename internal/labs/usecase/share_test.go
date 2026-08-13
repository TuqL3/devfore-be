package usecase

import (
	"testing"
	"time"
)

// The daily pick is the whole promise of the feature: two people comparing times
// have to have been handed the same fault. Every way of getting it wrong is
// quiet — a pick that drifts by timezone splits the day in two, a pick that
// leaves the range panics on a Tuesday, and a pick that walks the list in order
// hands out the scenarios alphabetically for a week.
func TestDailyIndexIsStableForOneDay(t *testing.T) {
	const n = 7
	day := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	want := DailyIndex(day, n)

	// Every instant inside that UTC day, whatever clock it is read on.
	for _, at := range []time.Time{
		day,
		day.Add(23*time.Hour + 59*time.Minute),
		day.Add(12 * time.Hour).In(time.FixedZone("ICT", 7*3600)),
		day.Add(3 * time.Hour).In(time.FixedZone("PST", -8*3600)),
	} {
		if got := DailyIndex(at, n); got != want {
			t.Errorf("DailyIndex(%s) = %d, want %d — the day would split in two",
				at.Format(time.RFC3339), got, want)
		}
	}
}

// A viewer in Hanoi and a viewer in Berlin looking at the same moment must see
// the same challenge, even when their local dates disagree. 2026-08-13T22:00Z is
// already the 14th in Hanoi.
func TestDailyIndexFollowsUTCNotTheViewer(t *testing.T) {
	at := time.Date(2026, 8, 13, 22, 0, 0, 0, time.UTC)
	hanoi := at.In(time.FixedZone("ICT", 7*3600))
	if hanoi.Format(time.DateOnly) == at.Format(time.DateOnly) {
		t.Fatal("test is not testing anything: pick a time where the local date differs")
	}
	if DailyIndex(hanoi, 5) != DailyIndex(at, 5) {
		t.Error("the pick moved with the viewer's clock")
	}
}

func TestDailyIndexStaysInRange(t *testing.T) {
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 400; i++ {
		for _, n := range []int{1, 2, 3, 7, 40} {
			got := DailyIndex(day.AddDate(0, 0, i), n)
			if got < 0 || got >= n {
				t.Fatalf("DailyIndex day+%d, n=%d = %d, out of range", i, n, got)
			}
		}
	}
}

// An empty field has no pick. -1 rather than 0 so a caller that forgets to check
// indexes out of bounds loudly instead of reading the first row of nothing.
func TestDailyIndexRefusesAnEmptyField(t *testing.T) {
	for _, n := range []int{0, -1} {
		if got := DailyIndex(time.Now(), n); got != -1 {
			t.Errorf("DailyIndex(n=%d) = %d, want -1", n, got)
		}
	}
}

// Consecutive days must not walk the list in order, and must not sit still. Both
// failures look fine on any single day and are only visible across a week.
func TestDailyIndexScattersAcrossDays(t *testing.T) {
	const n = 5
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	seen := map[int]int{}
	consecutive := 0
	prev := -99
	for i := 0; i < 60; i++ {
		got := DailyIndex(day.AddDate(0, 0, i), n)
		seen[got]++
		if got == prev+1 {
			consecutive++
		}
		prev = got
	}

	if len(seen) < n {
		t.Errorf("only %d of %d scenarios ever came up in 60 days: %v", len(seen), n, seen)
	}
	// A perfect +1 walk would score 59. Anything under half is noise, not a walk.
	if consecutive > 30 {
		t.Errorf("%d of 60 days stepped to the next scenario in order — the pick is a counter", consecutive)
	}
	for idx, count := range seen {
		if count > 30 {
			t.Errorf("scenario %d came up %d of 60 days", idx, count)
		}
	}
}
