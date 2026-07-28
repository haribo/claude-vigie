package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// sampleRetention is how many recent token samples we keep per session.
const sampleRetention = 30

// AddSample records an output-token sample for a session and prunes samples
// beyond the retention window.
func (s *Store) AddSample(ctx context.Context, sessionID, at string, outputTokens int64) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO token_samples (session_id, at, output_tokens) VALUES (?, ?, ?)
		 ON CONFLICT(session_id, at) DO UPDATE SET output_tokens = excluded.output_tokens`,
		sessionID, at, outputTokens); err != nil {
		return fmt.Errorf("adding sample for %s: %w", sessionID, err)
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM token_samples WHERE session_id = ? AND at NOT IN (
			SELECT at FROM token_samples WHERE session_id = ? ORDER BY at DESC LIMIT ?
		)`, sessionID, sessionID, sampleRetention); err != nil {
		return fmt.Errorf("pruning samples for %s: %w", sessionID, err)
	}
	return nil
}

// LastSampleAt returns the timestamp of the most recent sample, or "" if none.
func (s *Store) LastSampleAt(ctx context.Context, sessionID string) (string, error) {
	var at string
	err := s.db.QueryRowContext(ctx,
		`SELECT at FROM token_samples WHERE session_id = ? ORDER BY at DESC LIMIT 1`, sessionID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("last sample for %s: %w", sessionID, err)
	}
	return at, nil
}

// ListSamples returns up to limit output-token samples for a session newer than
// since (an RFC3339 timestamp; "" for no lower bound), oldest first. Sample
// timestamps are UTC, so a lexical comparison is chronological.
func (s *Store) ListSamples(ctx context.Context, sessionID, since string, limit int) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT output_tokens FROM token_samples WHERE session_id = ? AND at > ? ORDER BY at DESC LIMIT ?`,
		sessionID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("listing samples for %s: %w", sessionID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scanning sample: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating samples: %w", err)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}
