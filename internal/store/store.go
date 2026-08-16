// Package store persists fleet state in an embedded SQLite database: the
// current state of each Claude Code session plus an append-only event log.
// It uses the pure-Go modernc.org/sqlite driver so the daemon links no CGO
// (see ADR-0002). Only the daemon imports this package.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"sync"

	// Register the pure-Go SQLite driver under the name "sqlite".
	_ "modernc.org/sqlite"
)

// Store is a handle to the SQLite-backed fleet database.
type Store struct {
	db *sql.DB
	// applyMu serializes the read-modify-write cycle of ApplySession. See its
	// doc comment for why a process mutex is the right scope here.
	applyMu sync.Mutex
}

// Open opens (creating if needed) the SQLite database at path, applies
// pending migrations, and returns a ready Store.
func Open(path string) (*Store, error) {
	// Create the file ourselves, before SQLite can, so the database that persists
	// between runs is never world-readable — not even for the microseconds between
	// SQLite creating it and the chmod below. A pre-existing file is untouched
	// here and tightened by restrictToOwner (#526).
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		_ = f.Close()
	} else if !errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("creating %s: %w", path, err)
	}

	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("opening sqlite %s: %w", path, err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating: %w", err)
	}
	if err := restrictToOwner(path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// restrictToOwner makes the database readable by its owner alone.
//
// The fleet's shared token is stored in this file (daemon.serve: SetMeta "token"),
// and SQLite creates it with 0666 & ~umask — 0644 on a default umask, so every
// local account on the daemon host could read the secret and, with it, post
// reports or set the retention to 1ns and wipe the session table. The client has
// always written its copy at 0600 (internal/config); the daemon holds the copy
// that matters and did not (#526).
//
// Three files, not one: `-wal` holds committed pages, and both sidecars exist by
// the time Open returns because the DSN sets journal_mode(WAL). A chmod on the
// main file alone would leave the data readable.
//
// Existing databases are tightened too, not only new ones — the operators who
// need this are the ones already running one an earlier version created.
func restrictToOwner(path string) error {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		switch err := os.Chmod(p, 0o600); {
		case err == nil, errors.Is(err, fs.ErrNotExist):
			// Missing is fine: SQLite removes the sidecars on a clean close, so a
			// database opened read-only or not yet written may have neither.
		default:
			return fmt.Errorf("restricting %s: %w", p, err)
		}
	}
	return nil
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
