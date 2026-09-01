package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// RollUpTokens raises the session's mark and adds the counted growth to the
// (day, model) bucket, in one transaction, returning what was counted.
//
// The two used to be separate calls. `stats_daily` is never pruned and never
// recomputed, so a mark that advanced while the daily write failed lost that
// growth for good: the mark said it had been counted, and nothing had. Rare — the
// insert fails only on a write error — and invisible, because nothing recorded
// which day needed `vigied stats-repair` pointed at it (#669).
//
// Together, a failure leaves the mark where it was and the next report counts the
// same growth again. That is the property worth having: the mark's meaning is
// "already in stats_daily", and it should never be true ahead of the fact.
func (s *Store) RollUpTokens(ctx context.Context, sessionID string, total int64, day, model string) (int64, error) {
	if sessionID == "" || total <= 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var counted int64
	err = tx.QueryRowContext(ctx,
		`SELECT counted FROM session_token_mark WHERE session_id = ?`, sessionID).Scan(&counted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("reading token mark for %s: %w", sessionID, err)
	}
	if total <= counted {
		return 0, nil // already counted, or a regression
	}
	delta := total - counted

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO session_token_mark (session_id, counted) VALUES (?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET counted = excluded.counted`,
		sessionID, total); err != nil {
		return 0, fmt.Errorf("raising token mark for %s: %w", sessionID, err)
	}
	if err := addDailyTokens(ctx, tx, day, model, delta); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return delta, nil
}
