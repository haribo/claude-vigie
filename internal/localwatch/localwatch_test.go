package localwatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMarkIsLiveThenGoesStale(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	now := time.Now()
	if Live(now) {
		t.Error("no watcher has marked itself, yet the mark reads live")
	}
	if err := Mark(); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	// Pin the mark's timestamp before testing the edges. The kernel stamps a file
	// from a coarse cached clock that can lag time.Now() by milliseconds, so
	// comparing an exact window edge against an independently captured `now` is a
	// race between two clocks — one millisecond of lag is enough to fail, which is
	// what it did in CI while passing on every developer machine.
	if err := os.Chtimes(markPath(t, home), now, now); err != nil {
		t.Fatal(err)
	}

	if !Live(now) {
		t.Error("a watcher just marked itself and does not read live")
	}
	if !Live(now.Add(StaleAfter)) {
		t.Error("a mark exactly at the window edge should still be live")
	}
	if Live(now.Add(StaleAfter + time.Second)) {
		t.Error("a watcher that stopped beating still reads live")
	}
}

// TestMarkIsReadableAndPrivate: the file is state a human may look at, but it is
// written into the operator's home and gets the same permissions as the other
// state vigie writes there.
func TestMarkIsReadableAndPrivate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := Mark(); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	p := filepath.Join(home, ".local", "state", "vigie", "watcher")
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("the mark is not where the design says it is: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
}

// TestMarkRefreshes is what makes the freshness window mean anything: a second
// Mark must move the mtime forward, otherwise a running watcher would go stale.
func TestMarkRefreshes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := Mark(); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	p := filepath.Join(home, ".local", "state", "vigie", "watcher")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if Live(time.Now()) {
		t.Fatal("the backdated mark should be stale before the refresh")
	}
	if err := Mark(); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	if !Live(time.Now()) {
		t.Error("re-marking did not refresh the mark")
	}
}

// TestFutureMarkIsNotLive: a clock step or a copied-in file must not grant
// indefinite liveness — the fallback (do the work yourself) is always the safe
// answer.
func TestFutureMarkIsNotLive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := Mark(); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	p := filepath.Join(home, ".local", "state", "vigie", "watcher")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if Live(time.Now()) {
		t.Error("a mark dated in the future reads live")
	}
}

// markPath is where Mark writes, as the design document fixes it. Tests that need
// to control the timestamp use it rather than repeating the path.
func markPath(t *testing.T, home string) string {
	t.Helper()
	return filepath.Join(home, ".local", "state", "vigie", "watcher")
}
