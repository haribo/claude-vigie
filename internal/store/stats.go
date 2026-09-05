package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DailyStat is one UTC day's aggregated activity for a model. stats_daily is
// never pruned, so it retains history beyond the session-retention window.
type DailyStat struct {
	Day            string
	Model          string
	OutputTokens   int64
	WorkingSeconds int64
	WaitingSeconds int64
	IdleSeconds    int64
}

// execer is what both a *sql.DB and a *sql.Tx satisfy, so the statement below has
// one home whether it runs on its own or inside `RollUpTokens`'s transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// querier is what both a *sql.DB and a *sql.Tx satisfy for reads, so a query has
// one home whether it runs on its own or inside a transaction.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// addDailyTokens adds a positive output-token delta to the (day, model) bucket.
// Non-positive deltas are ignored.
func addDailyTokens(ctx context.Context, db execer, day, model string, delta int64) error {
	if delta <= 0 {
		return nil
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO stats_daily (day, model, output_tokens) VALUES (?, ?, ?)
		 ON CONFLICT(day, model) DO UPDATE SET output_tokens = output_tokens + excluded.output_tokens`,
		day, model, delta)
	if err != nil {
		return fmt.Errorf("adding daily tokens: %w", err)
	}
	return nil
}

// AddDailyStatusSeconds adds a duration (seconds) to a status bucket for
// (day, model). Statuses without a time bucket (e.g. ended) and non-positive
// durations are ignored.
func (s *Store) AddDailyStatusSeconds(ctx context.Context, day, model, status string, secs int64) error {
	return addDailyStatusSeconds(ctx, s.db, day, model, status, secs)
}

// addDailyStatusSeconds is the statement itself, so it has one home whether it
// runs on its own or inside `AppendEventClosingInterval`'s transaction (#737).
func addDailyStatusSeconds(ctx context.Context, db execer, day, model, status string, secs int64) error {
	if secs <= 0 {
		return nil
	}
	// Fixed queries per status (no dynamic column names) so the SQL is static.
	var query string
	switch status {
	case "working":
		query = `INSERT INTO stats_daily (day, model, working_seconds) VALUES (?, ?, ?)
		         ON CONFLICT(day, model) DO UPDATE SET working_seconds = working_seconds + excluded.working_seconds`
	case "waiting":
		query = `INSERT INTO stats_daily (day, model, waiting_seconds) VALUES (?, ?, ?)
		         ON CONFLICT(day, model) DO UPDATE SET waiting_seconds = waiting_seconds + excluded.waiting_seconds`
	case "idle":
		query = `INSERT INTO stats_daily (day, model, idle_seconds) VALUES (?, ?, ?)
		         ON CONFLICT(day, model) DO UPDATE SET idle_seconds = idle_seconds + excluded.idle_seconds`
	default:
		return nil
	}
	if _, err := db.ExecContext(ctx, query, day, model, secs); err != nil {
		return fmt.Errorf("adding daily %s seconds: %w", status, err)
	}
	return nil
}

// AddDailyTokens adds a delta on its own connection.
//
// No production code calls it: `RollUpTokens` is the only path tokens take into
// this table (#669). It stays because it is the store's public way to write that
// bucket, and `internal/daemon`'s stats-repair tests seed through it from another
// package, where the unexported helper is out of reach. Seeding them through
// `RollUpTokens` instead would write token marks those tests are not about.
func (s *Store) AddDailyTokens(ctx context.Context, day, model string, delta int64) error {
	return addDailyTokens(ctx, s.db, day, model, delta)
}

// ListDailyStats returns rows with day >= sinceDay (inclusive; "" for all),
// oldest first. Days are UTC "YYYY-MM-DD", so a lexical comparison is chronological.
func (s *Store) ListDailyStats(ctx context.Context, sinceDay string) ([]DailyStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT day, model, output_tokens, working_seconds, waiting_seconds, idle_seconds
		 FROM stats_daily WHERE day >= ? ORDER BY day, model`, sinceDay)
	if err != nil {
		return nil, fmt.Errorf("listing daily stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []DailyStat
	for rows.Next() {
		var d DailyStat
		if err := rows.Scan(&d.Day, &d.Model, &d.OutputTokens,
			&d.WorkingSeconds, &d.WaitingSeconds, &d.IdleSeconds); err != nil {
			return nil, fmt.Errorf("scanning daily stat: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating daily stats: %w", err)
	}
	return out, nil
}

// SetDailyTokens replaces the output-token figure of one (day, model) bucket and
// returns what it held before, plus whether the row existed.
//
// It exists because stats_daily is never recomputed: a day cannot be rebuilt from
// session rows, which retention has long deleted. A value corrupted before the
// rollup was made safe (#432) can therefore only be corrected deliberately, by an
// operator who decides what the right figure is. Nothing calls this on its own —
// a large day is not, by itself, wrong. See docs/design/token-rollup.md.
func (s *Store) SetDailyTokens(ctx context.Context, day, model string, tokens int64) (int64, bool, error) {
	var before int64
	err := s.db.QueryRowContext(ctx,
		`SELECT output_tokens FROM stats_daily WHERE day = ? AND model = ?`, day, model).Scan(&before)
	existed := true
	if errors.Is(err, sql.ErrNoRows) {
		existed = false
	} else if err != nil {
		return 0, false, fmt.Errorf("reading daily tokens: %w", err)
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO stats_daily (day, model, output_tokens) VALUES (?, ?, ?)
		 ON CONFLICT(day, model) DO UPDATE SET output_tokens = excluded.output_tokens`,
		day, model, tokens); err != nil {
		return 0, false, fmt.Errorf("setting daily tokens: %w", err)
	}
	return before, existed, nil
}
