package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// RaiseTokenMark records that a session's cumulative output total is now `total`
// and returns how much of it had not been counted yet.
//
// The rollup writes into stats_daily, which is never pruned and never recomputed,
// so a wrong value there is permanent. Counting the growth of the session row —
// a counter the rollup does not own — meant that any regression made the next
// report look like the session's entire lifetime of fresh output. Comparing
// against a mark of its own makes a regression contribute nothing, whatever
// caused it, and makes real growth count exactly once (#432,
// docs/design/token-rollup.md).
//
// A total at or below the mark returns 0 and leaves the mark alone.
func (s *Store) RaiseTokenMark(ctx context.Context, sessionID string, total int64) (int64, error) {
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

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO session_token_mark (session_id, counted) VALUES (?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET counted = excluded.counted`,
		sessionID, total); err != nil {
		return 0, fmt.Errorf("raising token mark for %s: %w", sessionID, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return total - counted, nil
}
