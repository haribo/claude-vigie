package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRefreshHooksInstalls is the #355 happy path: the watcher refreshes its own
// leg at startup, writing settings.json and logging it.
func TestRefreshHooksInstalls(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("VIGIE_CONFIG", "") // production leg

	out := captureStderr(t, refreshHooks)
	if !strings.Contains(out, "hooks refreshed") {
		t.Errorf("expected a refresh log, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")); err != nil {
		t.Errorf("settings.json should have been written: %v", err)
	}
}

// TestRefreshHooksMalformedIsNonFatal is the #355 failure path: a malformed
// settings.json is logged and the watcher survives (never panics or exits).
func TestRefreshHooksMalformedIsNonFatal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("VIGIE_CONFIG", "")
	dir := filepath.Join(os.Getenv("HOME"), ".claude")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{ bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := captureStderr(t, refreshHooks) // must not panic
	if !strings.Contains(out, "refreshing hooks failed") {
		t.Errorf("a malformed settings.json should be logged as a non-fatal failure, got %q", out)
	}
}
