package repo

import (
	"context"
	"time"

	"github.com/devforge/be/internal/labs/domain"
)

// recoveredSQL is the one definition of "the service came back", written once
// and joined into both queries below.
//
// It has to agree with usecase.RecoveredAt, which decides the same thing in Go
// for the private report: every task of the lab has a passing answer, and the
// recovery is dated from the last of them. Two definitions of recovered would
// eventually disagree, and the shape of the disagreement is a leaderboard that
// ranks somebody the report screen says never fixed it.
//
// `done` counts passing answers and `total` counts the lab's tasks, so a lab
// with no tasks recovers never rather than always — a drill nobody can fail is
// not a drill.
const recoveredSQL = `
	  LEFT JOIN LATERAL (
	      SELECT max(a.answered_at) AS last_at,
	             count(*) FILTER (WHERE a.passed) AS done
	        FROM lab_answers a
	       WHERE a.session_id = s.id
	  ) ans ON true
	  LEFT JOIN LATERAL (
	      SELECT count(*) AS total FROM lab_tasks t WHERE t.lab_id = s.lab_id
	  ) tk ON true`

// downtimeSQL turns that into the two numbers the board ranks by. Truncated to
// whole seconds, the same unit the private report already speaks in.
const downtimeSQL = `
	    (tk.total > 0 AND ans.done = tk.total)                            AS recovered,
	    COALESCE(EXTRACT(EPOCH FROM (ans.last_at - s.started_at)), 0)::int AS downtime_seconds`

// SetShareToken publishes a report under a token, or takes it back down when
// token is empty. One statement for both directions: publishing and
// unpublishing differ by the value, not by the rules around it.
func (r *SessionRepo) SetShareToken(ctx context.Context, sessionID string, token string) error {
	var val any
	if token != "" {
		val = token
	}
	return r.db.WithContext(ctx).Exec(
		`UPDATE lab_sessions SET share_token = ? WHERE id = ?`, val, sessionID,
	).Error
}

// ShareToken reads back what a session is published under, empty when it is not.
func (r *SessionRepo) ShareToken(ctx context.Context, sessionID string) (string, error) {
	var token *string
	res := r.db.WithContext(ctx).Raw(
		`SELECT share_token FROM lab_sessions WHERE id = ?`, sessionID,
	).Scan(&token)
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", domain.ErrNotFound
	}
	if token == nil {
		return "", nil
	}
	return *token, nil
}

// SharedByToken is the public page's only query, and the only place in the
// codebase that answers without a caller identity.
//
// The column list is the security boundary. It reads the display name and never
// the email; it reads the scenario's title and never its break_script or
// reveal_md; it never touches command_log. A `SELECT *` here would leak the
// answer key the first time somebody added a column.
func (r *SessionRepo) SharedByToken(ctx context.Context, token string) (*domain.SharedDrill, error) {
	var row struct {
		Token           string
		LabSlug         string
		LabTitle        string
		IncidentID      int64
		IncidentTitle   string
		Player          string
		StartedAt       time.Time
		Recovered       bool
		DowntimeSeconds int
		RPS             int
	}
	res := r.db.WithContext(ctx).Raw(`
		SELECT s.share_token AS token,
		       l.slug        AS lab_slug,
		       l.title       AS lab_title,
		       i.id          AS incident_id,
		       i.title       AS incident_title,
		       u.username    AS player,
		       s.started_at,
		       i.rps,`+downtimeSQL+`
		  FROM lab_sessions s
		  JOIN labs           l ON l.id = s.lab_id
		  JOIN lab_incidents  i ON i.id = s.incident_id
		  JOIN users          u ON u.id = s.user_id`+recoveredSQL+`
		 WHERE s.share_token = ?`, token,
	).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNotFound
	}
	return &domain.SharedDrill{
		Token:           row.Token,
		LabSlug:         row.LabSlug,
		LabTitle:        row.LabTitle,
		IncidentID:      row.IncidentID,
		IncidentTitle:   row.IncidentTitle,
		Player:          row.Player,
		StartedAt:       row.StartedAt,
		Recovered:       row.Recovered,
		DowntimeSeconds: row.DowntimeSeconds,
		RPS:             row.RPS,
	}, nil
}

// DrillScenarios lists every playable fault on the platform, in a fixed order.
//
// The order is the contract, not a nicety: the daily pick is an index into this
// list, so a list that came back in a different order each call would hand two
// people two different scenarios on the same day. ORDER BY on the two ids, which
// never change for a row.
//
// Published drills only, active scenarios only — the same two conditions the
// War Room list and PickIncident already apply, asked here in one query because
// the pick needs both halves at once.
func (r *SessionRepo) DrillScenarios(ctx context.Context) ([]domain.DrillScenario, error) {
	out := []domain.DrillScenario{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT l.id    AS lab_id,
		       l.slug  AS lab_slug,
		       l.title AS lab_title,
		       i.id    AS incident_id,
		       i.rps
		  FROM labs l
		  JOIN lab_incidents i ON i.lab_id = l.id AND i.active
		 WHERE l.drill_status = 'published'
		 ORDER BY l.id, i.id`,
	).Scan(&out).Error
	return out, err
}

// DailyLeaders ranks the recoveries against one scenario since a moment.
//
// Only sessions that recovered are on the board — a drill that ran out of time
// has no time to rank, and putting it last under a made-up number would be the
// board inventing a result. Ties break on who started earlier, so the order is
// stable between two refreshes.
//
// Answers times, not costs. Turning a duration into requests failed is a rule
// with a guard on it (usecase.RequestsFailed), and the caller applies it to
// every row with the scenario's own rate — one definition of the cost, not one
// here and one there.
func (r *SessionRepo) DailyLeaders(
	ctx context.Context, incidentID int64, since time.Time, limit int,
) ([]domain.DrillLeader, error) {
	out := []domain.DrillLeader{}
	err := r.db.WithContext(ctx).Raw(`
		SELECT u.username AS player,`+downtimeSQL+`
		  FROM lab_sessions s
		  JOIN users u ON u.id = s.user_id`+recoveredSQL+`
		 WHERE s.incident_id = ?
		   AND s.started_at >= ?
		   AND tk.total > 0 AND ans.done = tk.total
		 ORDER BY downtime_seconds, s.started_at
		 LIMIT ?`, incidentID, since, limit,
	).Scan(&out).Error
	return out, err
}

// IncidentForLab reads one named scenario, but only if it is a live scenario of
// that lab.
//
// The lab id in the WHERE clause is the check, not a filter for speed: this is
// reached from a shared link, where the scenario id arrives from a stranger's
// URL. Without it, a link could point a start at any row in the table — including
// a retired one, or one belonging to a lab the caller never opened.
func (r *SessionRepo) IncidentForLab(
	ctx context.Context, labID, incidentID int64,
) (*domain.Incident, error) {
	var inc domain.Incident
	res := r.db.WithContext(ctx).Raw(
		`SELECT id, title, break_script, reveal_md, rps
		   FROM lab_incidents
		  WHERE id = ? AND lab_id = ? AND active`, incidentID, labID,
	).Scan(&inc)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrIncidentNotInLab
	}
	return &inc, nil
}
