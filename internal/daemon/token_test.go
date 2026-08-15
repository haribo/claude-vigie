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

	// #465 removed the --token flag, so the case asserting it won is gone with it:
	// a token on the command line is world-readable through /proc/PID/cmdline. The
	// three orderings that remain are unchanged.

	// The environment wins over the store: a token the operator set explicitly
	// beats one the daemon persisted for itself.
	t.Setenv("VIGIE_TOKEN", "env-tok")
	st0 := newStore()
	if err := st0.SetMeta(ctx, "token", "stored-tok"); err != nil {
		t.Fatal(err)
	}
	if tok, err := resolveToken(ctx, st0); err != nil || tok != "env-tok" {
		t.Errorf("env should win: %q, %v", tok, err)
	}
	// The stored token wins when there is no env.
	t.Setenv("VIGIE_TOKEN", "")
	st := newStore()
	if err := st.SetMeta(ctx, "token", "meta-tok"); err != nil {
		t.Fatal(err)
	}
	if tok, err := resolveToken(ctx, st); err != nil || tok != "meta-tok" {
		t.Errorf("stored token should win: %q, %v", tok, err)
	}
	// Otherwise a token is generated and persisted, so it is stable across runs.
	st2 := newStore()
	tok1, err := resolveToken(ctx, st2)
	if err != nil || len(tok1) != 64 { // 32 random bytes, hex-encoded
		t.Fatalf("generated token = %q (len %d), %v", tok1, len(tok1), err)
	}
	if tok2, _ := resolveToken(ctx, st2); tok2 != tok1 {
		t.Errorf("generated token not persisted: %q != %q", tok2, tok1)
	}

	// The old name is gone: a leftover FLEET_TOKEN must not quietly re-key a fleet.
	t.Setenv("FLEET_TOKEN", "legacy-tok")
	st3 := newStore()
	if err := st3.SetMeta(ctx, "token", "kept-tok"); err != nil {
		t.Fatal(err)
	}
	if tok, err := resolveToken(ctx, st3); err != nil || tok != "kept-tok" {
		t.Errorf("FLEET_TOKEN is no longer read: %q, %v", tok, err)
	}
}
