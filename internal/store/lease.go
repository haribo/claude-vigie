package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AcquireLease grants the singleton usage-fetch lease to holder if it is free,
// expired, or already held by holder, extending it by ttl. It returns whether
// the lease was acquired and its new expiry (RFC3339). A killed holder's lease
// is not released explicitly — it simply expires.
func (s *Store) AcquireLease(ctx context.Context, holder string, ttl time.Duration, now time.Time) (bool, string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var curHolder, curExpiry string
	err = tx.QueryRowContext(ctx, `SELECT holder, expires_at FROM usage_lease WHERE id = 1`).
		Scan(&curHolder, &curExpiry)
	free := errors.Is(err, sql.ErrNoRows)
	if err != nil && !free {
		return false, "", fmt.Errorf("reading lease: %w", err)
	}

	if !free && curHolder != holder {
		exp, perr := time.Parse(time.RFC3339, curExpiry)
		if perr == nil && now.Before(exp) {
			// Held by someone else and still valid: deny.
			if err := tx.Commit(); err != nil {
				return false, "", fmt.Errorf("commit: %w", err)
			}
			return false, "", nil
		}
	}

	newExpiry := now.Add(ttl).UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO usage_lease (id, holder, expires_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET holder = excluded.holder, expires_at = excluded.expires_at`,
		holder, newExpiry); err != nil {
		return false, "", fmt.Errorf("writing lease: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, "", fmt.Errorf("commit: %w", err)
	}
	return true, newExpiry, nil
}
