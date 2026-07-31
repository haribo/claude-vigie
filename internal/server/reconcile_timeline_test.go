package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/haribo/claude-fleet/internal/api"
)

// TestStatusReconcileTimeline replays both observers — hook events and watcher
// scans — through the real report → store → view path on an injected clock, and
// asserts the reconciled status at each tick. This is the layer that was missing
// (#203): the unit tests exercise each observer alone, but the reconciliation
// bugs (#190/#201) live in how the two interleave over time.
func TestStatusReconcileTimeline(t *testing.T) {
	srv := newTestServer(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	srv.clock = func() time.Time { return now }

	rfc := func() string { return now.UTC().Format(time.RFC3339) }
	advance := func(d time.Duration) { now = now.Add(d) }

	report := func(id string, r api.ReportRequest) {
		t.Helper()
		r.SessionID, r.Machine, r.Timestamp = id, "m", rfc()
		body, _ := json.Marshal(r)
		if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code >= http.StatusMultipleChoices {
			t.Fatalf("report %+v = %d", r, rec.Code)
		}
	}
	hook := func(id, event string) { report(id, api.ReportRequest{Event: event}) }
	watch := func(id, status string) { report(id, api.ReportRequest{Event: "watch", Status: status}) }

	statusOf := func(id string) string {
		t.Helper()
		rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
		var views []api.SessionView
		if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
			t.Fatal(err)
		}
		for _, v := range views {
			if v.ID == id {
				return v.Status
			}
		}
		t.Fatalf("session %q not found", id)
		return ""
	}
	assert := func(id, want string) {
		t.Helper()
		if got := statusOf(id); got != want {
			t.Errorf("[%s] status = %q, want %q", id, got, want)
		}
	}

	// #201 — a watcher-only session that finishes must fall back to idle, not
	// latch to working (the watcher retracts its own state).
	watch("wonly", "working")
	assert("wonly", "working")
	advance(30 * time.Second)
	watch("wonly", "idle")
	assert("wonly", "idle")

	// #190 — a hook turn stays working while the transcript is briefly quiet (the
	// watcher reports idle), then a hook Stop ends the turn.
	hook("turn", "UserPromptSubmit")
	assert("turn", "working")
	advance(11 * time.Second)
	watch("turn", "idle") // transcript quiet mid-turn
	assert("turn", "working")
	advance(2 * time.Second)
	hook("turn", "Stop")
	assert("turn", "idle")

	// #233 — a watcher *confirmation* of the hook's working must not transfer
	// ownership, or a later idle (Claude reasoning, transcript quiet) steals the
	// turn. Confirm working, then go quiet: it must stay working.
	hook("think", "UserPromptSubmit")
	assert("think", "working")
	watch("think", "working") // watcher confirms (the prompt was just written)
	advance(11 * time.Second)
	watch("think", "idle") // quiet while Claude reasons
	assert("think", "working")

	// waiting — hook-only; the watcher's blind idle must not clear it, but real
	// activity resumes it.
	hook("wait", "Notification")
	assert("wait", "waiting")
	advance(3 * time.Second)
	watch("wait", "idle")
	assert("wait", "waiting")
	hook("wait", "UserPromptSubmit")
	assert("wait", "working")

	// #206 — a permission notification is waiting (operator is the blocker); an
	// idle notification is just idle (finished, awaiting the next prompt).
	report("perm", api.ReportRequest{Event: "Notification", NotificationType: "permission_prompt"})
	assert("perm", "waiting")
	report("done", api.ReportRequest{Event: "Notification", NotificationType: "idle_prompt"})
	assert("done", "idle")

	// error — a watcher observation wins and clears on recovery.
	watch("err", "working")
	watch("err", "error")
	assert("err", "error")
	watch("err", "idle")
	assert("err", "idle")
}
