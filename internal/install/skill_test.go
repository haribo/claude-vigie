package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallSkillWritesADiscoverableSkill: the description is the signal Claude
// matches on to load the skill, so an empty or unparseable one would silently
// make the whole call feature undiscoverable (#391).
func TestInstallSkillWritesADiscoverableSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := InstallSkill()
	if err != nil {
		t.Fatalf("InstallSkill: %v", err)
	}
	want := filepath.Join(home, ".claude", "skills", "vigie-call", "SKILL.md")
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is the one we just wrote
	if err != nil {
		t.Fatalf("reading skill: %v", err)
	}
	body := string(data)

	// Frontmatter must be delimited and carry a non-empty description. Parsed
	// structurally rather than with a YAML library: the project carries no YAML
	// dependency, and adding one to read four lines would not earn its place.
	if !strings.HasPrefix(body, "---\n") {
		t.Fatal("skill does not start with frontmatter")
	}
	end := strings.Index(body[4:], "\n---\n")
	if end < 0 {
		t.Fatal("frontmatter is not terminated")
	}
	desc := ""
	for _, line := range strings.Split(body[4:4+end], "\n") {
		if v, ok := strings.CutPrefix(line, "description:"); ok {
			desc = strings.TrimSpace(v)
		}
	}
	if desc == "" {
		t.Error("empty description: the skill would never be matched")
	}
	if len(desc) > 1536 {
		t.Errorf("description is %d chars; it is truncated at 1536", len(desc))
	}
	// A plain YAML scalar must not open with a quote, or the value would be
	// parsed as a quoted string and truncated at the closing quote.
	if strings.HasPrefix(desc, `"`) || strings.HasPrefix(desc, "'") {
		t.Error("description starts with a quote; it must be a plain scalar")
	}
	// It must teach the exact command, and be honest about its reliability.
	if !strings.Contains(body, "vigie call") {
		t.Error("the skill never names the command it exists to teach")
	}
	if !strings.Contains(body, "best-effort") {
		t.Error("the skill must state plainly that the call is best-effort")
	}
}

// TestInstallSkillIsIdempotent: it is refreshed on every watcher start, so a
// second write must leave exactly one skill, byte-identical.
func TestInstallSkillIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := InstallSkill()
	if err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path) //nolint:gosec // path is the one we just wrote
	if _, err := InstallSkill(); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path) //nolint:gosec // same path
	if string(first) != string(second) {
		t.Error("a refresh changed the skill; it must be byte-identical")
	}
	entries, err := os.ReadDir(filepath.Join(home, ".claude", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("skills dir holds %d entries, want exactly 1", len(entries))
	}
}

// TestUninstallSkillRemovesItAndIsSafeTwice: an absent skill is not an error, so
// uninstalling twice (or on a machine that never installed) stays quiet.
func TestUninstallSkillRemovesItAndIsSafeTwice(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := InstallSkill(); err != nil {
		t.Fatal(err)
	}
	path, err := UninstallSkill()
	if err != nil {
		t.Fatalf("UninstallSkill: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("skill still present after uninstall: %v", err)
	}
	if _, err := UninstallSkill(); err != nil {
		t.Errorf("a second uninstall should be a no-op, got %v", err)
	}
}
