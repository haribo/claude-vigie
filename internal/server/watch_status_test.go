package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/version"
)

// #527. The server tells its two informants apart by a weak signal: a report
// carrying no status is taken for a hook, and a hook is believed on its word —
// its state is stamped `hook` and the watcher may no longer retract it.
//
// A `watch` report with no status falls in that branch. It is not a hook: the
// watcher's whole contribution *is* the status it inferred, so an empty one says
// nothing at all. Believed anyway, it invents `working` on a new session and
// locks it there for good — the #201 latch, reachable from one malformed request.
//
// #515 bounded the vocabularies (unknown event, unknown status). It did not cover
// the status being absent.

func watchWithout(status string) api.ReportRequest {
	return api.ReportRequest{
		SessionID: "s", Machine: "m", Event: "watch", Status: status,
		WatcherVersion: version.Version, WatcherCommit: version.Commit,
	}
}

func TestAWatchReportWithoutAStatusIsRefused(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(watchWithout(""))
	rec := do(t, srv, http.MethodPost, "/api/report", body, true)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a watch report with no status = %d, want 400", rec.Code)
	}

	// The part that matters: refusing is worthless if it wrote anyway. Nothing
	// may have been created, and no state stamped.
	list := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
	var views []api.SessionView
	if err := json.Unmarshal(list.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 0 {
		t.Errorf("the refused report still created a session: %+v", views)
	}
}

// An existing session must be left exactly as it was, not merely not created.
func TestARefusedWatchDoesNotTouchAnExistingSession(t *testing.T) {
	srv := newTestServer(t)
	ok, _ := json.Marshal(watchWithout("idle"))
	if rec := do(t, srv, http.MethodPost, "/api/report", ok, true); rec.Code >= http.StatusMultipleChoices {
		t.Fatalf("setup: %d", rec.Code)
	}

	bad, _ := json.Marshal(watchWithout(""))
	do(t, srv, http.MethodPost, "/api/report", bad, true)

	list := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
	var views []api.SessionView
	if err := json.Unmarshal(list.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Status != "idle" {
		t.Errorf("the refused report changed the session: %+v", views)
	}
}

// Hooks keep deriving, which is correct: a Stop carries no status and means idle
// by definition. Breaking that would be a far worse trade than the bug fixed.
func TestHooksStillDeriveTheirStatus(t *testing.T) {
	for _, c := range []struct{ event, want string }{
		{"UserPromptSubmit", "working"},
		{"Stop", "idle"},
		{"SessionEnd", "ended"},
	} {
		srv := newTestServer(t)
		body, _ := json.Marshal(api.ReportRequest{SessionID: "s", Machine: "m", Event: c.event})
		if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code >= http.StatusMultipleChoices {
			t.Fatalf("%s = %d", c.event, rec.Code)
		}
		list := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
		var views []api.SessionView
		if err := json.Unmarshal(list.Body.Bytes(), &views); err != nil {
			t.Fatal(err)
		}
		if len(views) != 1 || views[0].Status != c.want {
			t.Errorf("%s produced %+v, want status %q", c.event, views, c.want)
		}
	}
}
