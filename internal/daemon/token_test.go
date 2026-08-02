package daemon

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/haribo/claude-vigie/internal/store"
)

func TestResolveToken(t *testing.T) {
	ctx := context.Background()
	newStore := func() *store.Store {
		t.Helper()
		st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		return st
	}

	// The flag wins over the env and the store.
	t.Setenv("FLEET_TOKEN", "env-tok")
	if tok, err := resolveToken(ctx, newStore(), "flag-tok"); err != nil || tok != "flag-tok" {
		t.Errorf("flag should win: %q, %v", tok, err)
	}
	// The env wins when there is no flag.
	if tok, err := resolveToken(ctx, newStore(), ""); err != nil || tok != "env-tok" {
		t.Errorf("env should win: %q, %v", tok, err)
	}
	// The stored token wins when there is no flag and no env.
	t.Setenv("FLEET_TOKEN", "")
	st := newStore()
	if err := st.SetMeta(ctx, "token", "meta-tok"); err != nil {
		t.Fatal(err)
	}
	if tok, err := resolveToken(ctx, st, ""); err != nil || tok != "meta-tok" {
		t.Errorf("stored token should win: %q, %v", tok, err)
	}
	// Otherwise a token is generated and persisted, so it is stable across runs.
	st2 := newStore()
	tok1, err := resolveToken(ctx, st2, "")
	if err != nil || len(tok1) != 64 { // 32 random bytes, hex-encoded
		t.Fatalf("generated token = %q (len %d), %v", tok1, len(tok1), err)
	}
	if tok2, _ := resolveToken(ctx, st2, ""); tok2 != tok1 {
		t.Errorf("generated token not persisted: %q != %q", tok2, tok1)
	}
}
