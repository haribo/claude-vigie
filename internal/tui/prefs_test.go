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
