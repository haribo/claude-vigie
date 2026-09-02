package server

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/status"
	"github.com/haribo/claude-vigie/internal/store"
	"github.com/haribo/claude-vigie/internal/version"
)

// ADR-0012 retired `stalled`, and rows written before that release still carry
// the word. The vocabulary's rule is that an unrecognized status degrades rather
// than breaks — the ADR says that should be tested rather than assumed, because
// #464 is what an unranked status costs: a NaN comparator in the dashboard, which
// does not sort badly, it stops sorting.
func TestALegacyStalledRowStillRenders(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-2 * time.Second).Format(time.RFC3339)

	v := toView(store.Session{ID: "s", Status: "stalled", ReportedAt: fresh}, nil, now, true)

	if v.Status != "stalled" {
		t.Errorf("status = %q; the stored value is shown as it is, not rewritten", v.Status)
	}
	if v.Attention {
		t.Error("a legacy `stalled` still calls the operator — the attention set is waiting and error")
	}
	if want := len(status.Order); v.Rank != want {
		t.Errorf("rank = %d, want %d — an unknown status sorts last, never first and never unranked", v.Rank, want)
	}
	if status.Known("stalled") {
		t.Error("`stalled` is still in the vocabulary")
	}
}

// The other half: the daemon refuses to *accept* the word going forward. A
// watcher that has not been upgraded must not be able to write it back in.
func TestAReportCarryingStalledIsRefused(t *testing.T) {
	srv := newTestServer(t)
	body := `{"session_id":"s","event":"watch","status":"stalled","machine":"m","timestamp":"2026-09-02T12:00:00Z",` +
		`"watcher_version":"` + version.Version + `","watcher_commit":"` + version.Commit + `"}`

	req := httptest.NewRequest("POST", "/api/report", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleReport(w, req)

	// The reason, not just the code: the same body with `idle` answers 204, so a
	// refusal that named something else would mean this test proved nothing.
	if w.Code != 400 || !strings.Contains(w.Body.String(), "unknown status") {
		t.Errorf("got %d %q; want 400 refusing the status — a stale watcher must not write it back in",
			w.Code, strings.TrimSpace(w.Body.String()))
	}
}
