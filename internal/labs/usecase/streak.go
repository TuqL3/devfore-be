package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/devforge/be/internal/labs/domain"
)

// WeeklyDays is the window the week's board covers, today included.
const WeeklyDays = 7

// dayPicks maps each of the last n days to the scenario that was handed out on
// it — the same answer DailyIndex gives, asked for a stretch of dates at once.
//
// Rebuilt from the current list of scenarios rather than remembered. That is the
// known cost of having no schedule table: a scenario published or retired today
// changes the list's length, and every past day's pick shifts with it. Yesterday
// then looks unsolved to anybody who solved it. The alternative is a row per day
// written by a job, which is a table, a backfill and a thing to keep in sync —
// and the platform publishes a scenario about once a month.
func (l *Labs) dayPicks(ctx context.Context, now time.Time, n int) (map[string]int64, error) {
	all, err := l.repo.DrillScenarios(ctx)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, domain.ErrNoDailyDrill
	}
	out := make(map[string]int64, n)
	today := now.UTC().Truncate(24 * time.Hour)
	for i := range n {
		day := today.AddDate(0, 0, -i)
		out[day.Format(time.DateOnly)] = all[DailyIndex(day, len(all))].IncidentID
	}
	return out, nil
}

// Weekly ranks the last seven days by how many of them each person solved.
//
// Days solved first, total time only as a tie-break. Seven days is seven
// different faults, so a sum of seconds across them measures which week somebody
// drew as much as how fast they were — a number that looks precise and compares
// nothing. "Five of seven" is a claim that survives the scenarios being unequal.
func (l *Labs) Weekly(ctx context.Context, now time.Time) ([]domain.WeeklyLeader, error) {
	picks, err := l.dayPicks(ctx, now, WeeklyDays)
	if err != nil {
		return nil, err
	}
	since := now.UTC().Truncate(24 * time.Hour).AddDate(0, 0, -(WeeklyDays - 1))
	rows, err := l.repo.RecoveriesSince(ctx, since)
	if err != nil {
		return nil, err
	}

	type tally struct {
		days  map[string]int
		total int
	}
	byPlayer := map[string]*tally{}
	for _, r := range rows {
		// Only the day's own scenario counts. Somebody who went back and played
		// last Tuesday's fault today has done something worth doing, and it is not
		// the same thing as having been there on Tuesday.
		if picks[r.Day] != r.IncidentID {
			continue
		}
		t := byPlayer[r.Player]
		if t == nil {
			t = &tally{days: map[string]int{}}
			byPlayer[r.Player] = t
		}
		// Best attempt of that day, not the first and not the sum: a second run at
		// the same fault is practice, and counting both would rank persistence as
		// slowness.
		if prev, seen := t.days[r.Day]; !seen || r.DowntimeSeconds < prev {
			t.days[r.Day] = r.DowntimeSeconds
		}
	}

	out := make([]domain.WeeklyLeader, 0, len(byPlayer))
	for player, t := range byPlayer {
		total := 0
		for _, secs := range t.days {
			total += secs
		}
		out = append(out, domain.WeeklyLeader{
			Player: player, DaysSolved: len(t.days), TotalTime: total,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DaysSolved != out[j].DaysSolved {
			return out[i].DaysSolved > out[j].DaysSolved
		}
		if out[i].TotalTime != out[j].TotalTime {
			return out[i].TotalTime < out[j].TotalTime
		}
		// Name last, so two identical rows never swap places between refreshes.
		return out[i].Player < out[j].Player
	})
	if len(out) > DailyLeaderLimit {
		out = out[:DailyLeaderLimit]
	}
	return out, nil
}

// Streak counts one person's run of consecutive days solved.
//
// Today not being solved yet does not break it. The alternative is a streak that
// ends every midnight and is rebuilt every morning, which nobody would call a
// streak — so the count starts at today if today is done, and at yesterday
// otherwise. Missing yesterday as well is what ends it.
func (l *Labs) Streak(ctx context.Context, userID int64, now time.Time) (*domain.DrillStreak, error) {
	picks, err := l.dayPicks(ctx, now, ArchiveDays)
	if err != nil {
		return nil, err
	}
	since := now.UTC().Truncate(24 * time.Hour).AddDate(0, 0, -(ArchiveDays - 1))
	rows, err := l.repo.RecoveriesSince(ctx, since)
	if err != nil {
		return nil, err
	}

	solved := map[string]bool{}
	for _, r := range rows {
		if r.UserID == userID && picks[r.Day] == r.IncidentID {
			solved[r.Day] = true
		}
	}

	today := now.UTC().Truncate(24 * time.Hour)
	out := &domain.DrillStreak{SolvedToday: solved[today.Format(time.DateOnly)]}

	start := 0
	if !out.SolvedToday {
		start = 1
	}
	for i := start; i < ArchiveDays; i++ {
		if !solved[today.AddDate(0, 0, -i).Format(time.DateOnly)] {
			break
		}
		out.Current++
	}

	// The longest run anywhere in the window, which is the number worth keeping
	// after a streak breaks — "you once did nine" is the reason to start again.
	run := 0
	for i := range ArchiveDays {
		if solved[today.AddDate(0, 0, -i).Format(time.DateOnly)] {
			run++
			out.Longest = max(out.Longest, run)
		} else {
			run = 0
		}
	}
	return out, nil
}
