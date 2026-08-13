package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
	"github.com/haribo/claude-vigie/internal/version"
)

func postEvent(t *testing.T, srv *Server, req api.ReportRequest) {
	t.Helper()
	body, _ := json.Marshal(req)
	if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code != http.StatusNoContent {
		t.Fatalf("%s = %d, want 204 (body: %s)", req.Event, rec.Code, rec.Body)
	}
}

func sessionView(t *testing.T, srv *Server, id string) api.SessionView {
	t.Helper()
	rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
	var views []api.SessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, v := range views {
		if v.ID == id {
			return v
		}
	}
	t.Fatalf("session %s not found in %+v", id, views)
	return api.SessionView{}
}

// TestCallRaisedAndClearedBySession is the ADR-0010 contract: the session raises
// the call and the session clears it — by resuming work, or by ending. No action
// on vigie is involved anywhere in this flow (ADR-0007).
func TestCallRaisedAndClearedBySession(t *testing.T) {
	for _, clearing := range []string{"UserPromptSubmit", "SessionEnd"} {
		srv := newTestServer(t)
		postEvent(t, srv, api.ReportRequest{
			Event: "UserPromptSubmit", SessionID: "s1", Machine: "m", Timestamp: "2026-08-12T10:00:00Z",
		})
		postEvent(t, srv, api.ReportRequest{
			Event: "call", SessionID: "s1", Machine: "m",
			CallMessage: "backfill done", Timestamp: "2026-08-12T10:05:00Z",
		})

		v := sessionView(t, srv, "s1")
		if v.CallAt != "2026-08-12T10:05:00Z" || v.CallMessage != "backfill done" {
			t.Fatalf("%s: call not recorded: at=%q msg=%q", clearing, v.CallAt, v.CallMessage)
		}

		postEvent(t, srv, api.ReportRequest{
			Event: clearing, SessionID: "s1", Machine: "m", Timestamp: "2026-08-12T10:06:00Z",
		})
		v = sessionView(t, srv, "s1")
		if v.CallAt != "" || v.CallMessage != "" {
			t.Errorf("%s did not clear the call: at=%q msg=%q", clearing, v.CallAt, v.CallMessage)
		}
	}
}

// TestCallWithoutMessageIsStillACall: the event raises the call, not the text.
func TestCallWithoutMessageIsStillACall(t *testing.T) {
	srv := newTestServer(t)
	postEvent(t, srv, api.ReportRequest{
		Event: "call", SessionID: "s1", Machine: "m", Timestamp: "2026-08-12T10:05:00Z",
	})
	if v := sessionView(t, srv, "s1"); v.CallAt == "" {
		t.Errorf("call with no message was not recorded: %+v", v)
	}
}

// TestCallIsOrthogonalToStatus guards the ADR-0010 boundary and the latch it
// would otherwise reopen: a call must neither change the status nor take
// ownership of it. If it stamped the source as "hook", a watcher-set `working`
// could no longer be retracted by the watcher itself (the #201 failure mode).
func TestCallIsOrthogonalToStatus(t *testing.T) {
	srv := newTestServer(t)
	// The watcher owns `working` here.
	postEvent(t, srv, api.ReportRequest{
		Event: "watch", SessionID: "s1", Machine: "m", Status: "working",
		WatcherVersion: version.Version, WatcherCommit: version.Commit,
		Timestamp: "2026-08-12T10:00:00Z",
	})
	postEvent(t, srv, api.ReportRequest{
		Event: "call", SessionID: "s1", Machine: "m", Timestamp: "2026-08-12T10:01:00Z",
	})
	if v := sessionView(t, srv, "s1"); v.Status != "working" {
		t.Errorf("call changed the status to %q, want working", v.Status)
	}

	// The watcher must still be able to retract its own state.
	postEvent(t, srv, api.ReportRequest{
		Event: "watch", SessionID: "s1", Machine: "m", Status: "idle",
		WatcherVersion: version.Version, WatcherCommit: version.Commit,
		Timestamp: "2026-08-12T10:02:00Z",
	})
	v := sessionView(t, srv, "s1")
	if v.Status != "idle" {
		t.Errorf("status latched at %q — the call took ownership of it", v.Status)
	}
	if v.CallAt == "" {
		t.Error("the watcher's report erased the call; it is orthogonal and must survive")
	}
}

// TestCallChangesTheVisibleSignature guards the SSE delta gate (#258): the
// dashboards only refetch when the operator-visible state changed, so raising or
// clearing a call must move the signature — otherwise a call would sit unseen
// until the next unrelated change.
func TestCallChangesTheVisibleSignature(t *testing.T) {
	base := store.Session{ID: "s1", Status: "idle"}
	raised := base
	raised.CallAt, raised.CallMessage = "2026-08-12T10:00:00Z", "done"

	if visibleSignature(base) == visibleSignature(raised) {
		t.Error("raising a call left the signature unchanged; SSE would not fire")
	}
	reworded := raised
	reworded.CallMessage = "something else"
	if visibleSignature(raised) == visibleSignature(reworded) {
		t.Error("changing the call message left the signature unchanged")
	}
	if visibleSignature(base) != visibleSignature(store.Session{ID: "s1", Status: "idle"}) {
		t.Error("signature is not stable for an unchanged session")
	}
}

// TestWatchReportPreservesCall: the watcher re-reports every session every couple
// of seconds and carries no call field. Those reports must not erase a call.
func TestWatchReportPreservesCall(t *testing.T) {
	srv := newTestServer(t)
	postEvent(t, srv, api.ReportRequest{
		Event: "call", SessionID: "s1", Machine: "m",
		CallMessage: "look at me", Timestamp: "2026-08-12T10:00:00Z",
	})
	postEvent(t, srv, api.ReportRequest{
		Event: "watch", SessionID: "s1", Machine: "m", Status: "idle",
		WatcherVersion: version.Version, WatcherCommit: version.Commit,
		Timestamp: "2026-08-12T10:00:02Z",
	})
	if v := sessionView(t, srv, "s1"); v.CallMessage != "look at me" {
		t.Errorf("watch report erased the call: %+v", v)
	}
}
