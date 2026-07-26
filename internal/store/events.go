package store

import (
	"context"
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
