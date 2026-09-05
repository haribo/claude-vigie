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
	return appendEvent(ctx, s.db, e)
}

// appendEvent is the statement itself, so it has one home whether it runs on its
// own or inside `AppendEventClosingInterval`'s transaction (#737).
func appendEvent(ctx context.Context, db execer, e Event) error {
	_, err := db.ExecContext(ctx,
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
	return lastEvent(ctx, s.db, sessionID)
}

// lastEvent is the query itself, so the interval rollup can read the mark inside
// its own transaction rather than before it (#737).
func lastEvent(ctx context.Context, db querier, sessionID string) (Event, bool, error) {
	var e Event
	err := db.QueryRowContext(ctx,
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

// StatusInterval is one closed status interval, ready for its daily bucket. A
// Secs of zero or less means there is nothing to add — no previous event, a span
// that will not parse or does not move forward, or a status with no bucket.
type StatusInterval struct {
	Day    string
	Model  string
	Status string
	Secs   int64
}

// AppendEventClosingInterval appends e and adds the interval it closes to the
// daily status buckets, in one transaction. bucket turns the session's previous
// event — where that interval started — into the row to write, and is called
// with ok=false when the session has no previous event.
//
// The event log is the mark the next interval measures from, and the two writes
// used to be separate: the seconds were added first and the append was
// best-effort. A failed append left the mark on the old event, so the next hook
// remeasured a span whose first half was already counted, and since stats_daily
// is never recomputed the day stayed inflated for good (#737).
//
// Together, a failure leaves the mark where it was and the next hook counts the
// interval once — over a longer span, attributed to the older status. That is the
// same trade #669 made for output tokens, and for the same reason: a mark must
// never be true ahead of the fact.
//
// Reading the mark inside the transaction closes the concurrent door too. Two
// hooks for one session used to be able to read the same previous event and both
// add the interval; now the second is refused by SQLite rather than counted.
func (s *Store) AppendEventClosingInterval(ctx context.Context, e Event, bucket func(last Event, ok bool) StatusInterval) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	last, ok, err := lastEvent(ctx, tx, e.SessionID)
	if err != nil {
		return err
	}
	if iv := bucket(last, ok); iv.Secs > 0 {
		if err := addDailyStatusSeconds(ctx, tx, iv.Day, iv.Model, iv.Status, iv.Secs); err != nil {
			return err
		}
	}
	if err := appendEvent(ctx, tx, e); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
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
