package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/devforge/be/internal/labs/domain"
)

type SimRepo struct{ db *gorm.DB }

func NewSimRepo(db *gorm.DB) *SimRepo { return &SimRepo{db: db} }

// ScenarioByLab reads the authored half of a sim lab, addressed by lab id rather
// than by slug. That is deliberate: a session already running has to stay
// runnable if the course is unpublished halfway through it, and SpecBySlug —
// which is the way in — filters on published on purpose.
func (r *SimRepo) ScenarioByLab(ctx context.Context, labID int64) (*domain.Scenario, error) {
	// Scanned into a struct rather than into a bare []byte: gorm reads a pointer
	// to a slice as "many rows", and the column would arrive one byte at a time.
	var row struct{ SimScenario []byte }
	res := r.db.WithContext(ctx).Raw(
		`SELECT sim_scenario FROM labs WHERE id = ?`, labID,
	).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrLabNotFound
	}
	if len(row.SimScenario) == 0 {
		return nil, domain.ErrNotSimLab
	}
	var sc domain.Scenario
	if err := json.Unmarshal(row.SimScenario, &sc); err != nil {
		return nil, fmt.Errorf("đọc kịch bản mô phỏng của lab %d: %w", labID, err)
	}
	return &sc, nil
}

// LatestRun is the newest run of a session — what grading reads, and what the
// next run inherits its number and its warm caches from. Both questions are the
// same row, so there is no separate count to fall out of step with it.
func (r *SimRepo) LatestRun(ctx context.Context, sessionID string) (*domain.SimRun, error) {
	var row simRunRow
	res := r.db.WithContext(ctx).Raw(
		`SELECT run_index, pipeline, result, created_at
		   FROM sim_runs
		  WHERE session_id = ?
		  ORDER BY run_index DESC
		  LIMIT 1`, sessionID,
	).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrNoSimRun
	}
	return row.decode()
}

// Runs is every run of a session, oldest first, so a student can see what
// changed between two presses of Run.
func (r *SimRepo) Runs(ctx context.Context, sessionID string) ([]domain.SimRun, error) {
	rows := []simRunRow{}
	if err := r.db.WithContext(ctx).Raw(
		`SELECT run_index, pipeline, result, created_at
		   FROM sim_runs
		  WHERE session_id = ?
		  ORDER BY run_index`, sessionID,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.SimRun, 0, len(rows))
	for i := range rows {
		run, err := rows[i].decode()
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, nil
}

// SaveRun stores one press of Run. The run number is decided by the caller
// because the engine already used it as part of its seed — a number picked here
// could disagree with the result being written next to it. The unique index is
// what settles two presses racing, and the loser is reported rather than retried
// under a new number: a retry would store a run nobody asked for.
func (r *SimRepo) SaveRun(ctx context.Context, sessionID string, run *domain.SimRun) error {
	result, err := json.Marshal(run.Result)
	if err != nil {
		return fmt.Errorf("ghi lượt chạy %d: %w", run.RunIndex, err)
	}
	err = r.db.WithContext(ctx).Exec(
		`INSERT INTO sim_runs (session_id, run_index, pipeline, result)
		 VALUES (?, ?, ?, ?)`,
		sessionID, run.RunIndex, run.Pipeline, string(result),
	).Error
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == uniqueViolation {
		return domain.ErrRunRaced
	}
	return err
}

// simRunRow is the stored shape, with the result still as bytes. Kept apart from
// domain.SimRun so the jsonb column is decoded in exactly one place.
type simRunRow struct {
	RunIndex  int
	Pipeline  string
	Result    []byte
	CreatedAt time.Time
}

func (row *simRunRow) decode() (*domain.SimRun, error) {
	run := &domain.SimRun{
		RunIndex:  row.RunIndex,
		Pipeline:  row.Pipeline,
		CreatedAt: row.CreatedAt,
	}
	if err := json.Unmarshal(row.Result, &run.Result); err != nil {
		return nil, fmt.Errorf("đọc kết quả lượt chạy %d: %w", row.RunIndex, err)
	}
	return run, nil
}
