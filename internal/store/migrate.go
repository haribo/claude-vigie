package store

import (
	"embed"
	"fmt"
	"sort"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrate applies every embedded migration not yet recorded, in filename
// order, each in its own transaction. Migrations are immutable once shipped.
func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
        name       TEXT PRIMARY KEY,
        applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
    )`); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var applied bool
		if err := s.db.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = ?)`, name,
		).Scan(&applied); err != nil {
			return fmt.Errorf("checking migration %s: %w", name, err)
		}
		if applied {
			continue
		}
		if err := s.applyMigration(name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyMigration(name string) error {
	sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("reading migration %s: %w", name, err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx for %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(string(sqlBytes)); err != nil {
		return fmt.Errorf("applying %s: %w", name, err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("recording %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", name, err)
	}
	return nil
}
