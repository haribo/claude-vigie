// Package store persists fleet state in an embedded SQLite database: the
// current state of each Claude Code session plus an append-only event log.
// It uses the pure-Go modernc.org/sqlite driver so the daemon links no CGO
// (see ADR-0002). Only the daemon imports this package.
package store

import (
	"database/sql"
	"fmt"

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
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite %s: %w", path, err)
	}

	// WAL for concurrent readers alongside the single writer; enforce foreign
	// keys (off by default in SQLite); wait instead of failing on a busy lock.
	pragmas := `PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`
	if _, err := db.Exec(pragmas); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("setting pragmas: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating: %w", err)
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	return s.db.Close()
}
