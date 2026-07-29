package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AppendEvent adds an entry to the append-only event log. The referenced
// session must already exist (enforced by a foreign key).
func (s *Store) AppendEvent(ctx context.Context, e Event) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events (session_id, event, status, created_at) VALUES (?, ?, ?, ?)`,
		e.SessionID, e.Event, e.Status, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("appending event for session %s: %w", e.SessionID, err)
	}
	return nil
}

// LastEvent returns the most recent event for a session; ok is false if the
// session has no events yet.
func (s *Store) LastEvent(ctx context.Context, sessionID string) (Event, bool, error) {
	var e Event
	err := s.db.QueryRowContext(ctx,
		`SELECT session_id, event, status, created_at FROM events
		 WHERE session_id = ? ORDER BY id DESC LIMIT 1`, sessionID).
		Scan(&e.SessionID, &e.Event, &e.Status, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Event{}, false, nil
	}
	if err != nil {
		return Event{}, false, fmt.Errorf("last event for session %s: %w", sessionID, err)
	}
	return e, true, nil
}

// ListEvents returns up to limit events for a session, most recent first.
func (s *Store) ListEvents(ctx context.Context, sessionID string, limit int) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, event, status, created_at FROM events
		 WHERE session_id = ? ORDER BY id DESC LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listing events for session %s: %w", sessionID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.SessionID, &e.Event, &e.Status, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating events: %w", err)
	}
	return out, nil
}
