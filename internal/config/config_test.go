package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	want := &Config{
		ServerURL: "http://localhost:8080",
		Token:     "s3cret",
		Machine:   "laptop",
	}

	path, err := Save(want)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if path == "" {
		t.Fatal("Save returned empty path")
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *got != *want {
		t.Fatalf("roundtrip mismatch: got %+v, want %+v", *got, *want)
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, err := Load(); err == nil {
		t.Fatal("expected error loading missing config, got nil")
	}
}

func TestFleetConfigOverride(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom.toml")
	t.Setenv("FLEET_CONFIG", custom)

	if p, err := Path(); err != nil || p != custom {
		t.Fatalf("Path() = %q, %v; want %q", p, err, custom)
	}

	want := &Config{ServerURL: "http://localhost:8099", Token: "dev", Machine: "box-dev"}
	if _, err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Errorf("override file not written: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *got != *want {
		t.Fatalf("roundtrip via FLEET_CONFIG: got %+v, want %+v", *got, *want)
	}
}

func TestMigratesLegacyJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	dir := filepath.Join(home, "claude-fleet")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(jsonPath,
		[]byte(`{"server_url":"https://api.example.org","token":"tok","machine":"box"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{ServerURL: "https://api.example.org", Token: "tok", Machine: "box"}
	if *got != want {
		t.Fatalf("migrated config = %+v, want %+v", *got, want)
	}

	// config.toml is written and the legacy config.json is removed.
	if _, err := os.Stat(filepath.Join(dir, "config.toml")); err != nil {
		t.Errorf("config.toml not written: %v", err)
	}
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Errorf("legacy config.json still present (stat err = %v)", err)
	}

	// A second load reads the TOML with no legacy file left.
	if _, err := Load(); err != nil {
		t.Fatalf("second Load after migration: %v", err)
	}
}
