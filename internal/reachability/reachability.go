// Package reachability records, on the machine itself, that the daemon did not
// answer — so a Claude Code hook can decline to wait on it again.
//
// A hook must not tax the session it observes (ADR-0005). It cannot ask the
// fleet whether the daemon is up, because asking is the cost it is trying to
// avoid, so the answer has to be waiting for it locally. The watcher keeps that
// answer fresh; a hook on a watcher-less machine keeps it for itself. See
// docs/design/unreachable-daemon.md (#578).
package reachability

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StaleAfter is how long a mark suppresses reports. It is twelve watcher beats,
// so on a machine running a watcher the mark is cleared the moment the daemon
// answers again and this value never governs anything. It bounds the other case:
// how long a watcher-less machine stays quiet after the daemon comes back.
const StaleAfter = 60 * time.Second

// dir is derived purely from HOME (not XDG_STATE_HOME) so the hook's and the
// watcher's environments always resolve to the same files — the same rule as
// presence (ADR-0006), compaction and the local watcher mark.
func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "vigie", "unreachable"), nil
}

// key identifies one daemon. The mark is per server, not per machine: `vigie
// hooks` installs one leg per VIGIE_CONFIG, and two legs point at two daemons
// whose reachability is independent — a stopped development server must not
// silence the production hooks (docs/design/unreachable-daemon.md § 4).
//
// The URL is hashed rather than escaped so the file name is bounded and holds no
// host name; the body carries the URL for a human reading the directory. The
// trailing slash is trimmed because that is what the callers' request builders
// do, so `http://h:8080` and `http://h:8080/` are one daemon here too.
func key(serverURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimRight(serverURL, "/")))
	return hex.EncodeToString(sum[:])[:16]
}

func pathFor(serverURL string) (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, key(serverURL)), nil
}

// Unreachable reports whether serverURL was found unreachable within StaleAfter
// of now.
//
// Every failure answers false: the caller's fallback is to attempt the request,
// so an unreadable mark must never be read as "do not bother". The unknown
// answer is the one that does the work — the mirror of localwatch.Live, where
// the unknown answer is also the one that does the work.
func Unreachable(serverURL string, now time.Time) bool {
	p, err := pathFor(serverURL)
	if err != nil {
		return false
	}
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	age := now.Sub(fi.ModTime())
	// Symmetric, like localwatch.Live: a mark a little ahead of now is ordinary
	// skew between two processes and still suppresses, while one far ahead (a
	// clock step, a file copied in) must not suppress reports indefinitely.
	return age <= StaleAfter && age >= -StaleAfter
}

// Mark records that serverURL did not answer at time at. Freshness is the file's
// mtime; the body is for a human reading the state directory and is never parsed.
//
// Call it for a transport failure only. An HTTP error response means the daemon
// answered — it is reachable, and the report was refused for its content, which
// is a different subject (docs/design/version-consistency.md).
func Mark(serverURL string, at time.Time, cause error) error {
	p, err := pathFor(serverURL)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}
	body := fmt.Sprintf("server %s\nsince %s\ncause %v\n", serverURL, at.UTC().Format(time.RFC3339), cause)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		return fmt.Errorf("writing unreachable mark: %w", err)
	}
	// The write stamped the file with the wall clock; the mark's age is `at`,
	// which the caller owns. Without this the injected clock would be decorative.
	if err := os.Chtimes(p, at, at); err != nil {
		return fmt.Errorf("dating unreachable mark: %w", err)
	}
	return nil
}

// Clear forgets the mark for serverURL, after it answered. Missing is not an
// error — the ordinary case is that there was nothing to forget.
func Clear(serverURL string) error {
	p, err := pathFor(serverURL)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clearing unreachable mark: %w", err)
	}
	return nil
}
