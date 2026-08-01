package daemon

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/haribo/claude-vigie/internal/store"
)

func tempStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestResolveTokenFlagWins(t *testing.T) {
	t.Setenv("FLEET_TOKEN", "envtok")
	got, err := resolveToken(context.Background(), tempStore(t), "flagtok")
	if err != nil {
		t.Fatal(err)
	}
	if got != "flagtok" {
		t.Errorf("token = %q, want flagtok", got)
	}
}

func TestResolveTokenEnv(t *testing.T) {
	t.Setenv("FLEET_TOKEN", "envtok")
	got, err := resolveToken(context.Background(), tempStore(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "envtok" {
		t.Errorf("token = %q, want envtok", got)
	}
}

func TestResolveTokenGeneratesAndPersists(t *testing.T) {
	t.Setenv("FLEET_TOKEN", "")
	st := tempStore(t)

	first, err := resolveToken(context.Background(), st, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 64 {
		t.Errorf("generated token length = %d, want 64 hex chars", len(first))
	}

	second, err := resolveToken(context.Background(), st, "")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("token not persisted across calls: %q != %q", first, second)
	}
}
