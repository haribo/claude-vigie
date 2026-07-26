package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound is returned when a requested session does not exist.
var ErrNotFound = errors.New("session not found")

const sessionColumns = `id, machine, project_dir, git_branch, model, status, last_tool,
	input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
	started_at, last_seen_at, ended_at`

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanSession(sc scanner) (Session, error) {
	var s Session
	err := sc.Scan(
		&s.ID, &s.Machine, &s.ProjectDir, &s.GitBranch, &s.Model, &s.Status, &s.LastTool,
		&s.Usage.InputTokens, &s.Usage.OutputTokens, &s.Usage.CacheCreationTokens, &s.Usage.CacheReadTokens,
		&s.StartedAt, &s.LastSeenAt, &s.EndedAt,
	)
	return s, err
}

// UpsertSession inserts a session or updates it if the ID already exists.
// StartedAt is preserved on update (only set at first insert).
func (s *Store) UpsertSession(ctx context.Context, sess Session) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions (
	id, machine, project_dir, git_branch, model, status, last_tool,
	input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
	started_at, last_seen_at, ended_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	machine = excluded.machine,
	project_dir = excluded.project_dir,
	git_branch = excluded.git_branch,
	model = excluded.model,
	status = excluded.status,
	last_tool = excluded.last_tool,
	input_tokens = excluded.input_tokens,
	output_tokens = excluded.output_tokens,
	cache_creation_tokens = excluded.cache_creation_tokens,
	cache_read_tokens = excluded.cache_read_tokens,
	last_seen_at = excluded.last_seen_at,
	ended_at = excluded.ended_at`,
		sess.ID, sess.Machine, sess.ProjectDir, sess.GitBranch, sess.Model, sess.Status, sess.LastTool,
		sess.Usage.InputTokens, sess.Usage.OutputTokens, sess.Usage.CacheCreationTokens, sess.Usage.CacheReadTokens,
		sess.StartedAt, sess.LastSeenAt, sess.EndedAt,
	)
	if err != nil {
		return fmt.Errorf("upserting session %s: %w", sess.ID, err)
	}
	return nil
}

// GetSession returns the session with the given ID, or ErrNotFound.
func (s *Store) GetSession(ctx context.Context, id string) (Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id)
	sess, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("getting session %s: %w", id, err)
	}
	return sess, nil
}

// ListSessions returns all sessions, most recently active first.
func (s *Store) ListSessions(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+` FROM sessions ORDER BY last_seen_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating sessions: %w", err)
	}
	return out, nil
}
