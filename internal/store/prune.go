package store

import (
	"context"
	"fmt"
	"time"
)

// staleWhere selects sessions considered stale by their last report, falling
// back to last_seen_at for rows written before last_report_at existed.
const staleWhere = `(CASE WHEN last_report_at != '' THEN last_report_at ELSE last_seen_at END) < ?`

// PruneSessions deletes sessions — and their events and token samples — whose
// last report is older than olderThan, bounding the database. It returns the
// number of sessions removed.
func (s *Store) PruneSessions(ctx context.Context, olderThan time.Duration, now time.Time) (int, error) {
	cutoff := now.Add(-olderThan).UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin prune: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	sub := `SELECT id FROM sessions WHERE ` + staleWhere
	if _, err := tx.ExecContext(ctx, `DELETE FROM token_samples WHERE session_id IN (`+sub+`)`, cutoff); err != nil {
		return 0, fmt.Errorf("pruning samples: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE session_id IN (`+sub+`)`, cutoff); err != nil {
		return 0, fmt.Errorf("pruning events: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE `+staleWhere, cutoff)
	if err != nil {
		return 0, fmt.Errorf("pruning sessions: %w", err)
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit prune: %w", err)
	}
	return int(n), nil
}
