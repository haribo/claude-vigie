package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/status"
	"github.com/haribo/claude-vigie/internal/version"
)

// #515. Four inputs were trusted because the caller holds the shared token. The
// threat model says the token means a trusted machine (deployment.md), so none of
// these was a hole in what is promised — they are the unbounded consequences of
// that promise, and each is cheap to bound.
//
// Each test below asserts the *rejection*: an acceptance test would keep passing
// if the check were deleted.

func postReportCode(t *testing.T, srv *Server, r api.ReportRequest) int {
	t.Helper()
	body, _ := json.Marshal(r)
	return do(t, srv, http.MethodPost, "/api/report", body, true).Code
}

// An unknown event fell through deriveStatus's default arm to `working` *and* was
// stamped hook-owned — which the watcher could then no longer retract (#201). A
// malformed `watch` with no status was enough.
func TestAnUnknownEventIsRefused(t *testing.T) {
	srv := newTestServer(t)
	// A `watch` declaring no build is refused by the drift gate before reaching
	// here (#384) — noted so a reader does not read its 409 as this check.
	if code := postReportCode(t, srv, api.ReportRequest{SessionID: "s", Machine: "m", Event: "watch"}); code != http.StatusConflict {
		t.Errorf("a watch report with no build was not refused by the drift gate: %d", code)
	}
	if code := postReportCode(t, srv, api.ReportRequest{SessionID: "s", Machine: "m", Event: "NotAThing"}); code != http.StatusBadRequest {
		t.Errorf("an unknown event was accepted: %d", code)
	}
	// And the events vigie really emits are all still accepted.
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PostToolUse", "Notification", "Stop", "PreCompact", "SessionEnd", "call"} {
		if code := postReportCode(t, srv, api.ReportRequest{SessionID: "s", Machine: "m", Event: ev}); code >= http.StatusMultipleChoices {
			t.Errorf("%s was refused: %d", ev, code)
		}
	}
}

// A status outside the vocabulary cannot be stored: every client renders from
// that vocabulary, so an unknown one is a row nothing knows how to draw.
func TestAnUnknownStatusIsRefused(t *testing.T) {
	srv := newTestServer(t)
	watch := func(st string) api.ReportRequest {
		return api.ReportRequest{
			SessionID: "s", Machine: "m", Event: "watch", Status: st,
			WatcherVersion: version.Version, WatcherCommit: version.Commit,
		}
	}
	if code := postReportCode(t, srv, watch("quantum")); code != http.StatusBadRequest {
		t.Errorf("an unknown status was accepted: %d", code)
	}
	for _, st := range status.All {
		if code := postReportCode(t, srv, watch(st)); code >= http.StatusMultipleChoices {
			t.Errorf("the known status %q was refused: %d", st, code)
		}
	}
}

// The lease exists so exactly one machine fetches; nothing checked it here, so
// any caller could overwrite the figure the whole fleet reads.
func TestUsageIsRefusedWithoutTheLease(t *testing.T) {
	srv := newTestServer(t)
	post := func(rep api.UsageReport) int {
		body, _ := json.Marshal(rep)
		return do(t, srv, http.MethodPost, "/api/usage", body, true).Code
	}

	// Nobody holds it yet.
	if code := post(api.UsageReport{FiveHourPct: 5, Holder: "m1"}); code != http.StatusConflict {
		t.Errorf("usage accepted with no lease held: %d", code)
	}

	lease, _ := json.Marshal(api.LeaseRequest{Holder: "m1"})
	if code := do(t, srv, http.MethodPost, "/api/usage/lease", lease, true).Code; code != http.StatusOK {
		t.Fatalf("acquiring the lease = %d", code)
	}
	if code := post(api.UsageReport{FiveHourPct: 5, Holder: "m2"}); code != http.StatusConflict {
		t.Errorf("a machine that does not hold the lease wrote the snapshot: %d", code)
	}
	if code := post(api.UsageReport{FiveHourPct: 5, Holder: ""}); code != http.StatusConflict {
		t.Errorf("a report with no holder was accepted: %d", code)
	}
	if code := post(api.UsageReport{FiveHourPct: 5, Holder: "m1"}); code != http.StatusNoContent {
		t.Errorf("the lease holder was refused: %d", code)
	}
}

// A percentage outside 0–100 renders as a gauge that means nothing, and the
// client cannot tell it apart from a real one.
func TestUsagePercentagesAreBounded(t *testing.T) {
	srv := newTestServer(t)
	lease, _ := json.Marshal(api.LeaseRequest{Holder: "m1"})
	do(t, srv, http.MethodPost, "/api/usage/lease", lease, true)

	for _, c := range []struct{ five, seven float64 }{{-5, 10}, {10, 900}, {101, 0}} {
		body, _ := json.Marshal(api.UsageReport{FiveHourPct: c.five, SevenDayPct: c.seven, Holder: "m1"})
		if code := do(t, srv, http.MethodPost, "/api/usage", body, true).Code; code != http.StatusBadRequest {
			t.Errorf("%v%% / %v%% was accepted: %d", c.five, c.seven, code)
		}
	}
	body, _ := json.Marshal(api.UsageReport{FiveHourPct: 0, SevenDayPct: 100, Holder: "m1"})
	if code := do(t, srv, http.MethodPost, "/api/usage", body, true).Code; code != http.StatusNoContent {
		t.Errorf("the bounds themselves were refused: %d", code)
	}
}

// The dashboard puts remote_url in an href, and HTML escaping does not stop a
// scheme from being followed. Validated at ingestion, so a bad value never
// reaches the store and no client has to remember to check.
func TestOnlyAnHTTPSRemoteURLIsStored(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want string
	}{
		{"https://claude.ai/code/abc", "https://claude.ai/code/abc"},
		{"javascript:alert(1)", ""},
		{"data:text/html;base64,PHNjcmlwdD4=", ""},
		{"http://claude.ai/code/abc", ""},
		{"https://", ""},
		{"", ""},
	} {
		if got := safeRemoteURL(c.raw); got != c.want {
			t.Errorf("safeRemoteURL(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}
