package config

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// #560. `config.toml` holds the fleet token, and `Save` writes it with
// `os.WriteFile(path, data, 0o600)`. That mode applies on **creation only**: a
// file that already exists keeps whatever bits it has, so a `config.toml` left at
// 0644 by an older build — or by an operator who created it by hand — stays
// readable by every local account on the machine, secret included.
//
// The daemon has had this fix since #526. `restrictToOwner`
// (internal/store/store.go) chmods the database and both WAL sidecars on every
// Open, existing files included, on the reasoning that "the operators who need
// this are the ones already running one an earlier version created". The comment
// in internal/store/permissions_test.go states that the client "already gets this
// right" — it did not, for exactly the case that matters.

// withUmask fixes the process umask for the test, so the assertion measures the
// code rather than the developer's shell.
func withUmask(t *testing.T, mask int) {
	t.Helper()
	old := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(old) })
}

// atConfigHome points Path() at a temporary directory.
func atConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("VIGIE_CONFIG", "")
	p, err := Path()
	if err != nil {
		t.Fatalf("resolving the config path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSaveCreatesTheConfigUnreadableByOthers(t *testing.T) {
	withUmask(t, 0)
	path := atConfigHome(t)

	if _, err := Save(&Config{ServerURL: "https://x", Token: "s3cret", Machine: "m"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("a new config.toml is %04o, want 0600 — it holds the fleet token", got)
	}
}

func TestSaveTightensAConfigThatAlreadyExists(t *testing.T) {
	withUmask(t, 0)
	path := atConfigHome(t)

	// What an older build, or a hand-rolled file, leaves behind.
	if err := os.WriteFile(path, []byte("server_url = \"https://old\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(&Config{ServerURL: "https://x", Token: "s3cret", Machine: "m"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("an existing config.toml is still %04o after Save, want 0600 — "+
			"every local account can read the token (#560)", got)
	}
}
