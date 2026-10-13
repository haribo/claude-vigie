package config

import (
	"path/filepath"
	"testing"
)

// On macOS os.UserConfigDir is ~/Library/Application Support whatever
// XDG_CONFIG_HOME says, so every test that isolated itself through the variable
// read and wrote the operator's real config instead (ADR-0018, #869).
func TestUserDirFollowsXDGOnMacOS(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Setenv("XDG_CONFIG_HOME", "")
	if d, err := UserDir(); err != nil || d != filepath.Join(home, ".config") {
		t.Errorf("unset: UserDir() = %q, %v; want %q", d, err, filepath.Join(home, ".config"))
	}

	xdg := filepath.Join(home, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if d, err := UserDir(); err != nil || d != xdg {
		t.Errorf("set: UserDir() = %q, %v; want %q", d, err, xdg)
	}

	// As os.UserConfigDir: a relative value is an error, not a path under the cwd.
	t.Setenv("XDG_CONFIG_HOME", "relative/dir")
	if d, err := UserDir(); err == nil {
		t.Errorf("relative: UserDir() = %q, want an error", d)
	}
}
