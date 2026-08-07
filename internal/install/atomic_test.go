package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallAtomicNoTempLeftover checks the #355 atomic write: the merge lands
// as settings.json with no stray temp file left in the directory.
func TestInstallAtomicNoTempLeftover(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := Install([]string{"SessionStart"}, "/opt/vigie", "", 5); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(os.Getenv("HOME"), ".claude")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "settings.json" {
		t.Errorf("dir should hold only settings.json, got %v", names)
	}
}

// TestInstallRejectsMalformed is the #355 failure path: a malformed settings.json
// makes Install fail and — since it errors before the write — leaves the file
// untouched (no corruption).
func TestInstallRejectsMalformed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(os.Getenv("HOME"), ".claude")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "settings.json")
	bad := []byte("{ not json ")
	if err := os.WriteFile(p, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install([]string{"SessionStart"}, "/opt/vigie", "", 5); err == nil {
		t.Error("Install should fail on malformed settings.json")
	}
	got, _ := os.ReadFile(p)
	if string(got) != string(bad) {
		t.Errorf("malformed file must be left untouched, got %q", got)
	}
}

// TestInstallRefreshesEventSet is the #355 refresh: re-installing with a larger
// default event set adds the new hook to vigie's leg.
func TestInstallRefreshesEventSet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := Install([]string{"SessionStart"}, "/opt/vigie", "", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := Install([]string{"SessionStart", "PreCompact"}, "/opt/vigie", "", 5); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".claude", "settings.json"))
	if !strings.Contains(string(data), "report --event=PreCompact") {
		t.Errorf("re-install should add the new PreCompact hook:\n%s", data)
	}
}
