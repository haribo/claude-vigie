package store

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// #526. The shared token lives in this database (`SetMeta(ctx, "token", …)`), and
// Open never set a mode on the file SQLite creates. With a default umask that is
// 0644 — every local account on the daemon host can read the fleet's secret, and
// with it post reports or set the retention to 1ns and wipe the session table.
//
// The client writes config.toml at 0600 because it holds the same secret — though
// only on creation, which #560 fixed there too. The daemon holds the copy that
// matters.
//
// Three files carry it, not one: `-wal` holds committed pages, and both sidecars
// exist as soon as Open returns because the DSN sets journal_mode(WAL).

// withUmask fixes the process umask for the test, so the assertion measures the
// code rather than the developer's shell — at umask 077 these tests would pass
// against a database that has no mode set at all.
func withUmask(t *testing.T, mask int) {
	t.Helper()
	old := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(old) })
}

// dbFiles returns the mode of every file in dir, keyed by name.
func dbFiles(t *testing.T, dir string) map[string]os.FileMode {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]os.FileMode{}
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = fi.Mode().Perm()
	}
	return out
}

func assertOwnerOnly(t *testing.T, dir string) {
	t.Helper()
	files := dbFiles(t, dir)
	if len(files) < 3 {
		t.Fatalf("expected the database and both sidecars, got %v", files)
	}
	for name, mode := range files {
		if mode&0o077 != 0 {
			t.Errorf("%s is %v — readable by other accounts on the host, and it carries the fleet's token", name, mode)
		}
	}
}

// A fresh database, opened under a permissive umask.
func TestANewDatabaseIsOwnerOnly(t *testing.T) {
	withUmask(t, 0o022)
	dir := t.TempDir()

	st, err := Open(filepath.Join(dir, "vigie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SetMeta(context.Background(), "token", "the-fleet-secret"); err != nil {
		t.Fatal(err)
	}
	assertOwnerOnly(t, dir)
}

// The upgrade path, which is the one that matters: the operators who need this
// are already running a database an earlier version created world-readable.
func TestAnExistingWorldReadableDatabaseIsTightened(t *testing.T) {
	withUmask(t, 0o022)
	dir := t.TempDir()
	path := filepath.Join(dir, "vigie.db")

	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetMeta(context.Background(), "token", "the-fleet-secret"); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	// Put it back the way an earlier version left it.
	for name := range dbFiles(t, dir) {
		//nolint:gosec // 0644 on purpose: this is the state an earlier version left behind, and what the fix must undo
		if err := os.Chmod(filepath.Join(dir, name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	st2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	assertOwnerOnly(t, dir)
}

// The guard for the guard: without a fixed umask these tests would pass on a
// developer running `umask 077` whatever the code does.
func TestTheUmaskIsFixedByTheTest(t *testing.T) {
	withUmask(t, 0o022)
	current := syscall.Umask(0)
	syscall.Umask(current)
	if current != 0o022 {
		t.Fatalf("umask = %#o, want 0o022 — the assertions above would measure the shell, not the code", current)
	}
}
