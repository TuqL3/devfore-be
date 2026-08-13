package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"hash/fnv"
	"io"
	"time"

	"github.com/devforge/be/internal/labs/domain"
)

// DailyLeaderLimit bounds the board. Twenty is a screen; the number nobody wants
// to read is the four hundredth slowest recovery of the day.
const DailyLeaderLimit = 20

// DailyIndex chooses today's scenario out of n, from the date alone.
//
// The date and nothing else — no counter, no last-chosen row, no random source.
// That is what lets two people on two machines compare times without the server
// having to remember which one it told first, and it is why the choice survives a
// restart, a redeploy and a database restored from last night's dump.
//
// UTC, and only the date part. A pick that rolled over at each viewer's local
// midnight would mean somebody in Hanoi and somebody in Berlin spent an hour on
// different faults while both screens said "today".
//
// FNV-1a over the formatted date rather than the day number, because consecutive
// day numbers walk the list in order and the platform would hand out its
// scenarios alphabetically for a week.
func DailyIndex(day time.Time, n int) int {
	if n <= 0 {
		return -1
	}
	h := fnv.New32a()
	_, _ = io.WriteString(h, day.UTC().Format(time.DateOnly))
	return int(h.Sum32() % uint32(n))
}

// dailyTTL is how stale the day's board may be.
//
// The two routes with no account behind them are the ones a link drops a crowd
// onto, and this one runs two queries per request. Fifteen seconds bounds that
// at two queries per fifteen seconds however many people arrive, and is short
// enough that somebody who just handed in finds themselves on the board while
// still looking at it.
const dailyTTL = 15 * time.Second

// Daily is the drill everybody gets today, and how everybody has done on it.
//
// The pick is made from the full list of published scenarios each call, not
// stored. Nothing to keep in sync, nothing to backfill, and a scenario an admin
// retires disappears from tomorrow without a job having to notice. The cost is
// real and worth naming: publishing or retiring a scenario mid-day reshuffles the
// list, so today's pick can change under people who already played it.
func (l *Labs) Daily(ctx context.Context, now time.Time) (*domain.DailyDrill, error) {
	return l.dayOf(ctx, now, true)
}

// ArchiveDays is how far back a past board may be asked for.
//
// A bound rather than none: the pick is a pure function of the date, so without
// one a crawler could walk to the year 1970 and every step is two queries.
const ArchiveDays = 90

// Day is one past day's challenge and its board, for the archive.
//
// Refuses the future outright. The pick is computable for any date, so an
// unguarded endpoint would hand out tomorrow's scenario to anybody who asked —
// and the whole point of the daily is that nobody has seen it first.
func (l *Labs) Day(ctx context.Context, day, now time.Time) (*domain.DailyDrill, error) {
	day = day.UTC().Truncate(24 * time.Hour)
	today := now.UTC().Truncate(24 * time.Hour)
	if day.After(today) || day.Before(today.AddDate(0, 0, -ArchiveDays)) {
		return nil, domain.ErrNoDailyDrill
	}
	return l.dayOf(ctx, day, day.Equal(today))
}

func (l *Labs) dayOf(ctx context.Context, at time.Time, isToday bool) (*domain.DailyDrill, error) {
	// Only today is cached. A past board is read once by a crawler and rarely by
	// anybody else, and keeping one of each in memory buys nothing.
	if isToday {
		if hit := l.cachedDaily(at); hit != nil {
			return hit, nil
		}
	}
	all, err := l.repo.DrillScenarios(ctx)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, domain.ErrNoDailyDrill
	}
	pick := all[DailyIndex(at, len(all))]

	utc := at.UTC()
	midnight := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	leaders, err := l.repo.DailyLeaders(
		ctx, pick.IncidentID, midnight, midnight.AddDate(0, 0, 1), DailyLeaderLimit)
	if err != nil {
		return nil, err
	}
	// The cost of each outage, worked out here rather than in the query: it is the
	// same authored-rate rule the private report uses, and one definition of it is
	// the difference between two screens agreeing and two screens nearly agreeing.
	for i := range leaders {
		leaders[i].RequestsFailed = RequestsFailed(
			time.Duration(leaders[i].DowntimeSeconds)*time.Second, pick.RPS)
	}
	out := &domain.DailyDrill{
		Day:        midnight.Format(time.DateOnly),
		LabSlug:    pick.LabSlug,
		LabTitle:   pick.LabTitle,
		IncidentID: pick.IncidentID,
		Leaders:    leaders,
	}
	if isToday {
		l.storeDaily(at, out)
	}
	return out, nil
}

// cachedDaily answers the last board if it is still fresh and still about today.
//
// The day is part of the check, not just the age: a board cached at 23:59:58
// describes yesterday's scenario, and serving it for another thirteen seconds
// would hand two people two different challenges across the rollover.
//
// A copy goes out, not the cached pointer. Handing callers the shared slice
// means the first one to sort or append it edits what everybody else reads.
func (l *Labs) cachedDaily(now time.Time) *domain.DailyDrill {
	l.dailyMu.RLock()
	defer l.dailyMu.RUnlock()
	if l.daily == nil || now.Sub(l.dailyAt) > dailyTTL {
		return nil
	}
	if l.daily.Day != now.UTC().Format(time.DateOnly) {
		return nil
	}
	out := *l.daily
	out.Leaders = append([]domain.DrillLeader(nil), l.daily.Leaders...)
	return &out
}

func (l *Labs) storeDaily(now time.Time, d *domain.DailyDrill) {
	l.dailyMu.Lock()
	defer l.dailyMu.Unlock()
	l.daily, l.dailyAt = d, now
}

// Share publishes the caller's own finished drill and answers with its link
// token. Pressing it twice answers the same token — a second press is somebody
// who lost the link, not somebody asking for a second page.
//
// Two refusals, both of them about what the page would contain rather than about
// permission. A session still running has no downtime to show and the fault is
// still the thing being worked out; an ordinary lab never drew a fault at all.
func (l *Labs) Share(ctx context.Context, sessionID string, userID int64) (string, error) {
	s, err := l.Owned(ctx, sessionID, userID)
	if err != nil {
		return "", err
	}
	if s.IncidentID == nil {
		return "", domain.ErrNotDrill
	}
	if s.Status == domain.StatusRunning {
		return "", domain.ErrStillRunning
	}

	existing, err := l.repo.ShareToken(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}

	token, err := newShareToken()
	if err != nil {
		return "", err
	}
	if err := l.repo.SetShareToken(ctx, sessionID, token); err != nil {
		return "", err
	}
	return token, nil
}

// Unshare takes the page back down. The token is dropped rather than kept and
// flagged: somebody who publishes and then thinks better of it is asking for the
// link to stop working, and a link that still resolves to a row marked hidden is
// one bug away from resolving to the page again.
//
// Anybody holding the old link gets the same 404 as a token that never existed.
// Republishing later mints a new one, so the old link stays dead.
func (l *Labs) Unshare(ctx context.Context, sessionID string, userID int64) error {
	if _, err := l.Owned(ctx, sessionID, userID); err != nil {
		return err
	}
	return l.repo.SetShareToken(ctx, sessionID, "")
}

// AdminUnshare takes somebody else's published report down.
//
// The lever that exists because the others cannot. The public page prints a
// display name under this platform's own domain, and usernames are only checked
// for shape at sign-up — three to thirty-two alphanumerics, which a slur fits
// inside comfortably. No word list closes that: it would be incomplete in two
// languages on the first day. What closes it is being able to remove a page in
// one request, from a link somebody sends you.
//
// Same effect as the owner pressing "take it down": the token is dropped, the
// old link is dead, and republishing later mints a new one.
func (l *Labs) AdminUnshare(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", domain.ErrNotFound
	}
	return l.repo.ClearShareToken(ctx, token)
}

// Shared reads a published drill for anybody at all — the one route on the
// platform with no caller identity behind it. What it may contain is decided in
// the query, by naming every column; see SessionRepo.SharedByToken.
//
// The two derived numbers are finished here, both of them the same way the
// private report finishes them. A drill that never recovered reports no downtime
// rather than the gap to its last answer: the outage did not end, and a page
// showing a duration beside "out of time" is describing something that did not
// happen.
func (l *Labs) Shared(ctx context.Context, token string) (*domain.SharedDrill, error) {
	if token == "" {
		return nil, domain.ErrNotFound
	}
	d, err := l.repo.SharedByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if !d.Recovered {
		d.DowntimeSeconds = 0
		return d, nil
	}
	d.RequestsFailed = RequestsFailed(time.Duration(d.DowntimeSeconds)*time.Second, d.RPS)
	return d, nil
}

// newShareToken is 16 random bytes, which is the point: the token is the only
// thing standing between a URL and a page, so it has to be a secret rather than
// a name. Same generator and same alphabet as a session id — URL-safe, no
// padding, nothing that needs escaping when somebody pastes it into a chat.
func newShareToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate share token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
