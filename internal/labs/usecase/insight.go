package usecase

import (
	"context"
	"time"

	"github.com/devforge/be/internal/labs/domain"
)

// The read side of the admin screens. Thin on purpose: every one of these is a
// question the database can answer in one statement, and a layer that reshaped
// the answers would be a second place for the meaning to live.

// ContentHealth is everything the analysis screen shows at once.
//
// Four queries in one call rather than four endpoints, because the four are read
// together and compared against each other — a lab with a high drop rate and a
// task at 0% pass inside it is one finding, not two.
type ContentHealth struct {
	Tasks     []domain.TaskHealth
	Labs      []domain.LabHealth
	Incidents []domain.IncidentHealth
	Courses   []domain.CourseHealth
}

func (l *Labs) ContentHealth(ctx context.Context) (*ContentHealth, error) {
	tasks, err := l.repo.TaskHealth(ctx)
	if err != nil {
		return nil, err
	}
	labs, err := l.repo.LabHealth(ctx)
	if err != nil {
		return nil, err
	}
	incidents, err := l.repo.IncidentHealth(ctx)
	if err != nil {
		return nil, err
	}
	courses, err := l.repo.CourseHealth(ctx)
	if err != nil {
		return nil, err
	}
	return &ContentHealth{Tasks: tasks, Labs: labs, Incidents: incidents, Courses: courses}, nil
}

// Overview is the live screen: what is happening right now, and the window's
// totals beside it so "eleven sessions" has something to be eleven against.
type Overview struct {
	Counts   *domain.PlatformCounts
	Running  []domain.RunningSession
	MaxSlots int
	Since    time.Time
}

func (l *Labs) Overview(ctx context.Context, since time.Time) (*Overview, error) {
	counts, err := l.repo.PlatformCounts(ctx, since)
	if err != nil {
		return nil, err
	}
	running, err := l.grades.RunningSessions(ctx)
	if err != nil {
		return nil, err
	}
	return &Overview{
		Counts: counts, Running: running, MaxSlots: l.maxContainers, Since: since,
	}, nil
}

func (l *Labs) SharedReports(ctx context.Context, limit int) ([]domain.SharedReport, error) {
	return l.repo.SharedReports(ctx, limit)
}

// UserActivity is one person's page: who they are, what they attempted, and
// every pipeline they wrote.
//
// The shell history is deliberately NOT here. It is the most sensitive thing
// stored about somebody, and folding it into a page that loads on every visit
// would make reading it something that happens by accident. It has its own call,
// so opening one is a decision with an audit entry attached.
type UserActivity struct {
	Summary  *domain.UserSummary
	Sessions []domain.UserSession
	SimRuns  []domain.UserSimRun
}

func (l *Labs) UserActivity(ctx context.Context, userID int64) (*UserActivity, error) {
	summary, err := l.repo.UserSummary(ctx, userID)
	if err != nil {
		return nil, err
	}
	sessions, err := l.repo.UserSessions(ctx, userID, 50)
	if err != nil {
		return nil, err
	}
	runs, err := l.repo.UserSimRuns(ctx, userID, 50)
	if err != nil {
		return nil, err
	}
	return &UserActivity{Summary: summary, Sessions: sessions, SimRuns: runs}, nil
}

// SessionCommands is the shell history of one drill, for an admin.
//
// Returns the raw text rather than the parsed timeline: somebody looking at this
// is looking for what actually happened, and the parser drops lines it does not
// recognise as commands.
func (l *Labs) SessionCommands(ctx context.Context, sessionID string) (string, error) {
	return l.repo.CommandLog(ctx, sessionID)
}
