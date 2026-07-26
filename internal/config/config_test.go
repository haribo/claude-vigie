package config

import (
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
