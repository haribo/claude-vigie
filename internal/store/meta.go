package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetMeta returns the value for key and whether it exists.
func (s *Store) GetMeta(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("getting meta %s: %w", key, err)
	}
	return v, true, nil
}

// SetMeta inserts or updates the value for key.
func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("setting meta %s: %w", key, err)
	}
	return nil
}
