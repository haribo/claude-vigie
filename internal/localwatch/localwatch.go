// Package localwatch records, on the machine itself, that a watcher is running
// here. The server-side heartbeat (docs/design/watcher-liveness.md) answers the
// same question for the fleet, but a Claude Code hook cannot ask it: a hook must
// not make a network call to decide what to do.
//
// It exists so a hook can tell whether the watcher has already read this
// session's transcript — see docs/design/transcript-reads.md (#420).
package localwatch

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StaleAfter is how long a mark stays trustworthy: three missed beats at the
// watcher's 5 s heartbeat, the same window the TUI applies to the server-side
// heartbeat. A watcher that dies mid-session therefore stops being deferred to
// within one window, and hooks resume reading transcripts on their own.
const StaleAfter = 15 * time.Second

// path is derived purely from HOME (not XDG_STATE_HOME) so the hook's and the
// watcher's environments always resolve to the same file — the same rule as
// presence (ADR-0006) and compaction.
func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "vigie", "watcher"), nil
}

// Mark records that a watcher is alive now. Freshness is the file's mtime; the
// contents are for a human reading the state directory and are never parsed.
func Mark() error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}
	body := fmt.Sprintf("pid %d\n", os.Getpid())
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		return fmt.Errorf("writing watcher mark: %w", err)
	}
	return nil
}

// Live reports whether a watcher marked itself within StaleAfter of now. Every
// failure answers false: the caller's fallback is to do the work itself, so an
// unreadable mark must never be read as "someone else has it covered".
func Live(now time.Time) bool {
	p, err := path()
	if err != nil {
		return false
	}
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	age := now.Sub(fi.ModTime())
	// The window is symmetric. A mark a little ahead of `now` is ordinary skew —
	// the caller captured the time just before the watcher wrote the file, or the
	// two processes disagree by milliseconds — and must still read live. A mark
	// far ahead (a clock step, or a file copied in from elsewhere) is as
	// untrustworthy as an old one and must not grant liveness indefinitely.
	return age <= StaleAfter && age >= -StaleAfter
}
