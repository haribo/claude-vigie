package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// contextOf fetches the single session's context reading from /api/sessions.
func contextOf(t *testing.T, srv *Server) *int64 {
	t.Helper()
	rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
	var views []api.SessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("want 1 session, got %d", len(views))
	}
	return views[0].ContextTokens
}

func report(t *testing.T, srv *Server, req api.ReportRequest) {
	t.Helper()
	body, _ := json.Marshal(req)
	if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code != http.StatusNoContent {
		t.Fatalf("report = %d", rec.Code)
	}
}

// TestContextKnownMerge proves the known/unknown distinction survives the
// read-modify-write merge (#367, point A): a nil reading keeps the last known
// value (a hook event without context must not erase it), while a known 0
// (a just-cleared session) is applied as 0%, not dropped as "absent".
func TestContextKnownMerge(t *testing.T) {
	srv := newTestServer(t)

	// A watcher report carries a known 500k context.
	report(t, srv, api.ReportRequest{
		Event: "watch", SessionID: "s1", Machine: "m", ProjectDir: "/p",
		Status: "working", ContextTokens: ctxPtr(500_000), Timestamp: "2026-07-26T10:00:00Z",
	})
	if c := contextOf(t, srv); c == nil || *c != 500_000 {
		t.Fatalf("after known 500k, context = %v, want 500000", c)
	}

	// A later report without context (nil) must keep the last known value.
	report(t, srv, api.ReportRequest{
		Event: "UserPromptSubmit", SessionID: "s1", Machine: "m", ProjectDir: "/p",
		Timestamp: "2026-07-26T10:00:01Z",
	})
	if c := contextOf(t, srv); c == nil || *c != 500_000 {
		t.Errorf("a nil-context report erased the known value: %v, want 500000 kept", c)
	}

	// A known 0 (fresh context after a switch) is applied — the view shows 0%,
	// represented as a non-nil pointer to 0, not a dropped/unknown reading.
	report(t, srv, api.ReportRequest{
		Event: "watch", SessionID: "s1", Machine: "m", ProjectDir: "/p",
		Status: "idle", ContextTokens: ctxPtr(0), Timestamp: "2026-07-26T10:00:02Z",
	})
	c := contextOf(t, srv)
	if c == nil {
		t.Fatal("known 0 was dropped to unknown; want a non-nil 0 (0%)")
	}
	if *c != 0 {
		t.Errorf("context = %d, want 0", *c)
	}
}
