// Package compaction records that a Claude Code session started compacting its
// context, so the watcher can refine its status from an opaque `working` to
// `compacting` while it summarizes (ADR-0008, #342). Claude Code writes nothing
// to the transcript when compaction starts and its registry status enum is
// closed, so a PreCompact hook drops a marker here and the watcher reads it —
// the same hook-writes / watcher-reads split as session presence (ADR-0006).
package compaction

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Marker is the compaction-start record a PreCompact hook writes.
type Marker struct {
	Started string `json:"started"` // RFC3339, when compaction began
	Trigger string `json:"trigger"` // "auto" | "manual"
}

// dir is the marker directory, derived purely from HOME (not XDG_STATE_HOME) so
// the hook's and the watcher's environments always resolve to the same path —
// the same rule as presence (ADR-0006).
func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "vigie", "compacting"), nil
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

// Save records that a session started compacting (called at PreCompact).
func Save(sessionID string, m Marker) error {
	p, err := pathFor(sessionID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("creating compacting dir: %w", err)
	}
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encoding marker: %w", err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("writing marker: %w", err)
	}
	return nil
}

// Load returns the compaction marker for a session; ok is false if none exists.
func Load(sessionID string) (m Marker, ok bool, err error) {
	p, err := pathFor(sessionID)
	if err != nil {
		return Marker{}, false, err
	}
	data, err := os.ReadFile(p) //nolint:gosec // path is validated and confined to our compacting dir
	if os.IsNotExist(err) {
		return Marker{}, false, nil
	}
	if err != nil {
		return Marker{}, false, fmt.Errorf("reading marker: %w", err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return Marker{}, false, fmt.Errorf("parsing marker: %w", err)
	}
	return m, true, nil
}

// Remove deletes a session's marker (called by the watcher once the compaction
// has closed or the window expired). Missing is not an error.
func Remove(sessionID string) error {
	p, err := pathFor(sessionID)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing marker: %w", err)
	}
	return nil
}

// StartedAt parses the marker's start time; ok is false on a missing/bad value.
func (m Marker) StartedAt() (t time.Time, ok bool) {
	t, err := time.Parse(time.RFC3339, m.Started)
	return t, err == nil
}
