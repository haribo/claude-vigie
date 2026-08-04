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

func TestVigieConfigOverride(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom.toml")
	t.Setenv("VIGIE_CONFIG", custom)

	if p, err := Path(); err != nil || p != custom {
		t.Fatalf("Path() = %q, %v; want %q", p, err, custom)
	}
}

// TestVigieConfigTakesPrecedence: when both are set, VIGIE_CONFIG wins and
// FLEET_CONFIG is only a fallback (#289).
func TestVigieConfigTakesPrecedence(t *testing.T) {
	vigie := filepath.Join(t.TempDir(), "vigie.toml")
	t.Setenv("FLEET_CONFIG", filepath.Join(t.TempDir(), "fleet.toml"))
	t.Setenv("VIGIE_CONFIG", vigie)

	if p, err := Path(); err != nil || p != vigie {
		t.Fatalf("Path() = %q, %v; want VIGIE_CONFIG to win (%q)", p, err, vigie)
	}
	if got := EnvConfigPath(); got != vigie {
		t.Fatalf("EnvConfigPath() = %q, want %q", got, vigie)
	}
}

func TestMigratesLegacyJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	dir := filepath.Join(home, "vigie")
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

func TestLoadFallsBackToLegacyDir(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	// A config.toml written under the pre-rename ~/.config/claude-fleet directory.
	oldDir := filepath.Join(cfgHome, "claude-fleet")
	if err := os.MkdirAll(oldDir, 0o750); err != nil {
		t.Fatal(err)
	}
	body := "server_url = \"http://old:8080\"\ntoken = \"tok\"\nmachine = \"m\"\n"
	if err := os.WriteFile(filepath.Join(oldDir, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load() // no vigie/config.toml exists → must read the legacy one
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ServerURL != "http://old:8080" || got.Token != "tok" {
		t.Errorf("legacy fallback not read: %+v", got)
	}
}
