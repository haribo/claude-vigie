package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/status"
	"github.com/haribo/claude-vigie/internal/version"
)

func scrape(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	MetricsHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics = %d", rec.Code)
	}
	return rec.Body.String()
}

func reportStatus(t *testing.T, srv *Server, id, st string) {
	t.Helper()
	body, _ := json.Marshal(api.ReportRequest{
		Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit,
		SessionID: id, Machine: "m", Status: st, Timestamp: "2026-07-31T12:00:00Z",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code >= http.StatusMultipleChoices {
		t.Fatalf("report %s/%s = %d", id, st, rec.Code)
	}
}

// TestGaugeDeclaresEveryStatus is #421: `Collect` tallies every status the API
// returns but emitted a series only for those in its own hand-written list, which
// had never gained `compacting` (#342). Those sessions were counted and then
// dropped, so the gauge could not add up.
func TestGaugeDeclaresEveryStatus(t *testing.T) {
	srv := newTestServer(t)
	RegisterStateCollector(srv.store, "no-such.db")
	t.Cleanup(func() { RegisterStateCollector(nil, "") })

	out := scrape(t)
	for _, s := range status.All {
		if want := fmt.Sprintf(`vigie_sessions{status=%q}`, s); !strings.Contains(out, want) {
			t.Errorf("/metrics has no series for %q — a session in that status would vanish", s)
		}
	}
}

// TestGaugeCountsEverySession is the invariant behind it: whatever mix of
// statuses is stored, the gauge's total equals the number of sessions. A status
// missing from the series list fails this even when every individual series looks
// plausible.
func TestGaugeCountsEverySession(t *testing.T) {
	srv := newTestServer(t)
	RegisterStateCollector(srv.store, "no-such.db")
	t.Cleanup(func() { RegisterStateCollector(nil, "") })

	// One session per status the watcher can report. `stale` and `ended` are
	// reconciled from staleness rather than reported, so they are left out here;
	// what matters is that no reported status goes uncounted.
	reported := []string{"working", "thinking", "compacting", "waiting", "idle", "error"}
	for i, s := range reported {
		reportStatus(t, srv, fmt.Sprintf("s%d", i), s)
	}

	out := scrape(t)
	line := regexp.MustCompile(`vigie_sessions\{status="[a-z]+"\} (\d+)`)
	total := 0
	for _, m := range line.FindAllStringSubmatch(out, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("unparsable gauge line %q", m[0])
		}
		total += n
	}
	if total != len(reported) {
		t.Errorf("gauge totals %d for %d sessions — some status is counted and then dropped", total, len(reported))
	}
}
