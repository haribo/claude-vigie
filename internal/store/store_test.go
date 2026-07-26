package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func sampleSession(id string) Session {
	return Session{
		ID:         id,
		Machine:    "laptop",
		ProjectDir: "/home/x/proj",
		GitBranch:  "main",
		Model:      "claude-opus-4-8",
		Status:     "working",
		Usage:      Usage{InputTokens: 100, OutputTokens: 50},
		StartedAt:  "2026-07-26T10:00:00Z",
		LastSeenAt: "2026-07-26T10:00:00Z",
	}
}

func TestMigrateIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	st1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := st1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	st2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open (migrate must be idempotent): %v", err)
	}
	_ = st2.Close()
}

func TestUpsertAndGetSession(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	sess := sampleSession("s1")
	if err := st.UpsertSession(ctx, sess); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := st.GetSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != sess {
		t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", got, sess)
	}

	// Update: new status and tokens, later last_seen. started_at must persist.
	upd := sess
	upd.Status = "idle"
	upd.Usage.OutputTokens = 999
	upd.LastSeenAt = "2026-07-26T11:00:00Z"
	upd.StartedAt = "SHOULD-BE-IGNORED"
	if err := st.UpsertSession(ctx, upd); err != nil {
		t.Fatalf("upsert update: %v", err)
	}

	got, err = st.GetSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Status != "idle" {
		t.Errorf("status = %q, want idle", got.Status)
	}
	if got.Usage.OutputTokens != 999 {
		t.Errorf("output tokens = %d, want 999", got.Usage.OutputTokens)
	}
	if got.StartedAt != sess.StartedAt {
		t.Errorf("started_at = %q, want preserved %q", got.StartedAt, sess.StartedAt)
	}
	if got.LastSeenAt != upd.LastSeenAt {
		t.Errorf("last_seen_at = %q, want %q", got.LastSeenAt, upd.LastSeenAt)
	}
}

func TestGetSessionNotFound(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.GetSession(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListSessions(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	a := sampleSession("a")
	a.LastSeenAt = "2026-07-26T10:00:00Z"
	b := sampleSession("b")
	b.LastSeenAt = "2026-07-26T12:00:00Z"
	if err := st.UpsertSession(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSession(ctx, b); err != nil {
		t.Fatal(err)
	}

	list, err := st.ListSessions(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	if list[0].ID != "b" || list[1].ID != "a" {
		t.Errorf("order = [%s, %s], want [b, a] (last_seen desc)", list[0].ID, list[1].ID)
	}
}

func TestAppendAndListEvents(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if err := st.UpsertSession(ctx, sampleSession("s1")); err != nil {
		t.Fatal(err)
	}

	e1 := Event{SessionID: "s1", Event: "SessionStart", Status: "working", CreatedAt: "2026-07-26T10:00:00Z"}
	e2 := Event{SessionID: "s1", Event: "Stop", Status: "idle", CreatedAt: "2026-07-26T10:05:00Z"}
	if err := st.AppendEvent(ctx, e1); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendEvent(ctx, e2); err != nil {
		t.Fatal(err)
	}

	list, err := st.ListEvents(ctx, "s1", 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	if list[0].Event != "Stop" {
		t.Errorf("first = %q, want Stop (most recent first)", list[0].Event)
	}
}

func TestAppendEventUnknownSession(t *testing.T) {
	st := openTestStore(t)
	err := st.AppendEvent(context.Background(), Event{
		SessionID: "ghost", Event: "Stop", CreatedAt: "2026-07-26T10:00:00Z",
	})
	if err == nil {
		t.Fatal("expected a foreign key error for an unknown session, got nil")
	}
}
