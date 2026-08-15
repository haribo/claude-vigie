package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #480. The preferences were written with os.WriteFile — truncate, then write —
// and a read failure fell back to defaults which the next keystroke stamped over
// the file. A corrupt file therefore became an unrecoverable one, with nothing
// said anywhere. This is what #479's loss would have looked like even without the
// test leak.

// prefsFileAt returns the path loadPrefs/savePrefs use, inside the test sandbox.
func prefsFileAt(t *testing.T) string {
	t.Helper()
	p, err := prefsPath()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func writePrefsFile(t *testing.T, content string) string {
	t.Helper()
	path := prefsFileAt(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The heart of it: a file that cannot be used must survive the session that could
// not use it.
func TestAnUnparsableFileIsNotOverwritten(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const corrupt = "this is not = valid toml ["
	path := writePrefsFile(t, corrupt)

	p := loadPrefs()
	if p.loadFailed == "" {
		t.Error("an unparsable file loaded without complaint")
	}
	if p.hideEnded != defaultPrefs().hideEnded {
		t.Error("the session should run on defaults")
	}

	// The keystroke that used to destroy it.
	p.hideEnded = !p.hideEnded
	savePrefs(p)

	after, err := os.ReadFile(path) //nolint:gosec // the path is our own temp dir
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != corrupt {
		t.Errorf("the file was overwritten:\n  %q", string(after))
	}
}

// An empty file is the nastier case: it parses cleanly into zero values, so
// nothing looks wrong while every preference silently changes.
func TestAnEmptyFileIsTreatedAsCorrupt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := writePrefsFile(t, "   \n\t\n")

	p := loadPrefs()
	if p.loadFailed == "" {
		t.Fatal("an empty file was accepted as a valid configuration")
	}
	savePrefs(p)

	after, err := os.ReadFile(path) //nolint:gosec // the path is our own temp dir
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(after)) != "" {
		t.Errorf("the empty file was replaced rather than kept: %q", string(after))
	}
}

// An unreadable file must not be replaced either — the contents are still there,
// and a permission problem is fixable.
func TestAnUnreadableFileIsNotOverwritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permissions do not deny anything")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := writePrefsFile(t, "hide_ended = true\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	p := loadPrefs()
	if p.loadFailed == "" {
		t.Error("an unreadable file loaded without complaint")
	}
	savePrefs(p)

	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path) //nolint:gosec // the path is our own temp dir
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "hide_ended = true\n" {
		t.Errorf("the unreadable file was replaced: %q", string(after))
	}
}

// The failure has to reach the operator, or "we kept your file" is a secret.
func TestTheSettingsTabSaysThePreferencesCouldNotBeRead(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writePrefsFile(t, "not = valid [")

	m := stubModel()
	m.width = 120
	m.prefs = loadPrefs()

	out := m.renderSettings()
	if !strings.Contains(out, "could not") && !strings.Contains(out, "cannot") {
		t.Errorf("the Settings tab says nothing about the unusable file:\n%s", out)
	}
	if !strings.Contains(out, "defaults") {
		t.Errorf("it does not say the session is running on defaults:\n%s", out)
	}
}

// A healthy file still round-trips, and saving still works — the guard must not
// have made the preferences read-only for everyone.
func TestAGoodFileStillSaves(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	p := loadPrefs() // creates the commented default
	if p.loadFailed != "" {
		t.Fatalf("a fresh install reported %q", p.loadFailed)
	}
	p.hideEnded = true
	p.columnOrder = []string{"status", "name"}
	savePrefs(p)

	again := loadPrefs()
	if again.loadFailed != "" {
		t.Fatalf("the file we just wrote cannot be read back: %s", again.loadFailed)
	}
	if !again.hideEnded {
		t.Error("hide_ended did not round-trip")
	}
	if strings.Join(again.columnOrder, ",") != "status,name" {
		t.Errorf("column order = %v, want [status name]", again.columnOrder)
	}
}

// The write goes through a temp file and a rename, which is what makes a TUI
// killed mid-save leave the previous file intact instead of a truncated one.
//
// Atomicity itself cannot be observed without killing a process at the right
// instant, so this asserts the mechanism that provides it: a rename replaces the
// directory entry, so the inode changes. Writing in place — the previous
// behavior — keeps it. That distinction is exactly the fix.
func TestTheWriteReplacesTheFileRatherThanTruncatingIt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	p := loadPrefs()
	path := prefsFileAt(t)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	p.hideEnded = true
	savePrefs(p)

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if sameFile(before, after) {
		t.Error("the file was written in place — a crash mid-write would truncate it")
	}
	if after.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 — the temp file's mode must carry over", after.Mode().Perm())
	}
}

// The directory keeps nothing but the preferences file: a temp file left behind
// would accumulate one per save.
func TestTheWriteLeavesNoDebris(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	p := loadPrefs()
	p.hideEnded = true
	savePrefs(p)

	dir := filepath.Dir(prefsFileAt(t))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "tui.toml" {
			t.Errorf("left %q behind — a temp file must be renamed or removed", e.Name())
		}
	}
}

// sameFile reports whether two stat results describe the same file on disk.
func sameFile(a, b os.FileInfo) bool { return os.SameFile(a, b) }
