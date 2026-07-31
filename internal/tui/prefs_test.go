package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadPrefsCreatesDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	p := loadPrefs()
	if !p.hideEnded || p.idleHideAfter != 0 {
		t.Errorf("defaults = %+v, want hideEnded=true idleHideAfter=0", p)
	}
	// The file should now exist with a commented template.
	path, _ := prefsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("default prefs file not created: %v", err)
	}
	if !strings.Contains(string(data), "idle_hide_after") || !strings.Contains(string(data), "#") {
		t.Errorf("default file missing key or comments:\n%s", data)
	}
}

func TestLoadPrefsParsesFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, _ := prefsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("hide_ended = false\nidle_hide_after = \"90m\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := loadPrefs()
	if p.hideEnded {
		t.Error("hide_ended=false not applied")
	}
	if p.idleHideAfter != 90*time.Minute {
		t.Errorf("idleHideAfter = %s, want 90m", p.idleHideAfter)
	}
}

func TestSortAndGroupPrefsRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	p := defaultPrefs()
	p.sortKey = sortTokens
	p.sortReversed = true
	p.groupBy = groupProject
	savePrefs(p)

	got := loadPrefs()
	if got.sortKey != sortTokens || !got.sortReversed || got.groupBy != groupProject {
		t.Errorf("round-trip = {sort:%d rev:%t group:%d}, want {tokens true project}",
			got.sortKey, got.sortReversed, got.groupBy)
	}
}

func TestSortAndGroupPrefsDefaultOnUnknown(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, _ := prefsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	// A file with no sort/group keys, plus a garbage sort name, must fall back to
	// the defaults rather than a bogus key.
	if err := os.WriteFile(path, []byte("sort_key = \"bogus\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := loadPrefs()
	if p.sortKey != sortLastSeen || p.groupBy != groupNone {
		t.Errorf("unknown/missing = {sort:%d group:%d}, want {last seen off}", p.sortKey, p.groupBy)
	}
}

func TestCyclePreset(t *testing.T) {
	if cyclePreset(0, 1) != 15*time.Minute {
		t.Error("forward from off should be 15m")
	}
	if cyclePreset(0, -1) != idlePresets[len(idlePresets)-1] {
		t.Error("backward from off should wrap to the last preset")
	}
}
