// Package store persists fleet state in an embedded SQLite database: the
// current state of each Claude Code session plus an append-only event log.
// It uses the pure-Go modernc.org/sqlite driver so the daemon links no CGO
// (see ADR-0002). Only the daemon imports this package.
package store

import (
	"database/sql"
	"fmt"
	"net/url"

	// Register the pure-Go SQLite driver under the name "sqlite".
	_ "modernc.org/sqlite"
)

// Store is a handle to the SQLite-backed fleet database.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path, applies
// pending migrations, and returns a ready Store.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("opening sqlite %s: %w", path, err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating: %w", err)
	}
	return s, nil
}

// dsn builds the driver DSN. The pragmas travel as `_pragma` query params so the
// modernc driver runs them on *every* pooled connection — not just the first.
// busy_timeout and foreign_keys are per-connection state: setting them once via
// db.Exec left every other connection the pool later opened with busy_timeout=0,
// so under concurrent writes those connections failed immediately with
// SQLITE_BUSY instead of waiting — surfacing as intermittent 500s (#372).
// journal_mode=WAL (persistent in the file header) gives concurrent readers
// alongside the single writer; busy_timeout makes contending writers wait.
func dsn(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(on)")
	q.Add("_pragma", "journal_mode(WAL)")
	return "file:" + path + "?" + q.Encode()
}

// Close releases the database handle.
func (s *Store) Close() error {
	return s.db.Close()
}
