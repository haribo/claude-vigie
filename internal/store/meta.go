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

// ListMeta returns every meta key/value. The table holds a handful of rows
// (fleet-wide flags and per-machine watch state), so callers filter in Go rather
// than with a LIKE prefix — the keys contain `_`, a LIKE wildcard, and an
// unescaped prefix match would quietly match the wrong keys (#384).
func (s *Store) ListMeta(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM meta`)
	if err != nil {
		return nil, fmt.Errorf("listing meta: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scanning meta: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating meta: %w", err)
	}
	return out, nil
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
