// Package presence links a Claude Code session to the OS process backing it, so
// the watcher can reliably tell a live session (idle for any duration) from a
// closed one — something transcript activity alone cannot do. A SessionStart
// hook records the mapping; the watcher checks whether the process is still
// alive. Linux only (it reads /proc). Client side.
package presence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Mapping records which process backs a session, captured at SessionStart.
// StartTime disambiguates a reused pid: a different process with the same pid
// has a different start time, so a dead session is never seen as alive.
type Mapping struct {
	PID       int    `json:"pid"`
	StartTime uint64 `json:"start_time"` // /proc/<pid>/stat field 22 (clock ticks since boot)
}

// dir is the session-mapping directory, derived purely from HOME (not
// XDG_STATE_HOME) so the reporter's hook environment and the watcher's systemd
// environment always resolve to the same path.
func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "claude-fleet", "sessions"), nil
}

func pathFor(sessionID string) (string, error) {
	if sessionID == "" || sessionID == "." || sessionID == ".." || strings.ContainsAny(sessionID, `/\`) {
		return "", fmt.Errorf("invalid session id %q", sessionID)
	}
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, sessionID+".json"), nil
}

// Save records the session→process mapping (called at SessionStart).
func Save(sessionID string, m Mapping) error {
	p, err := pathFor(sessionID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("creating sessions dir: %w", err)
	}
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encoding mapping: %w", err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("writing mapping: %w", err)
	}
	return nil
}

// Load returns the mapping recorded for a session; ok is false if none exists.
func Load(sessionID string) (m Mapping, ok bool, err error) {
	p, err := pathFor(sessionID)
	if err != nil {
		return Mapping{}, false, err
	}
	data, err := os.ReadFile(p) //nolint:gosec // path is validated and confined to our sessions dir
	if os.IsNotExist(err) {
		return Mapping{}, false, nil
	}
	if err != nil {
		return Mapping{}, false, fmt.Errorf("reading mapping: %w", err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return Mapping{}, false, fmt.Errorf("parsing mapping: %w", err)
	}
	return m, true, nil
}

// Delete removes a session mapping (called at SessionEnd); absent is not an error.
func Delete(sessionID string) error {
	p, err := pathFor(sessionID)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing mapping: %w", err)
	}
	return nil
}
