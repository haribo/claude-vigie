package report

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// TestCallPostsTheSessionsCall covers the raise half of #388: the call names the
// session from Claude Code's own environment handle and carries the message.
func TestCallPostsTheSessionsCall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(sessionIDEnv, "sess-42")

	var got api.ReportRequest
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	writeConfig(t, srv.URL)

	if err := Call("backfill done — 12k rows"); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if gotPath != "/api/report" {
		t.Errorf("posted to %s", gotPath)
	}
	if got.Event != "call" {
		t.Errorf("event = %q, want call", got.Event)
	}
	if got.SessionID != "sess-42" {
		t.Errorf("session = %q, want sess-42 (from %s)", got.SessionID, sessionIDEnv)
	}
	if got.CallMessage != "backfill done — 12k rows" {
		t.Errorf("message = %q", got.CallMessage)
	}
	if got.Timestamp == "" {
		t.Error("call carries no timestamp; it is what marks the call active")
	}
	// A call must not smuggle a status in: it is orthogonal to it (ADR-0010).
	if got.Status != "" {
		t.Errorf("call carried status %q, want none", got.Status)
	}
}

// TestCallWithoutMessage: the event raises the call, the text is optional.
func TestCallWithoutMessage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(sessionIDEnv, "sess-42")

	var got api.ReportRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	writeConfig(t, srv.URL)

	if err := Call(""); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got.Event != "call" || got.CallMessage != "" {
		t.Errorf("got event=%q message=%q, want a call with no message", got.Event, got.CallMessage)
	}
}

// TestCallOutsideASessionErrors: run outside Claude Code there is no session to
// call about. It must report the reason rather than post a session-less call —
// the caller still exits 0, so nothing fails.
func TestCallOutsideASessionErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(sessionIDEnv, "")

	posted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		posted = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	writeConfig(t, srv.URL)

	if err := Call("hello"); err == nil {
		t.Error("Call outside a session should report an error")
	}
	if posted {
		t.Error("Call posted a report with no session id")
	}
}
