package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/version"
)

var metricRe = regexp.MustCompile(`vigie_[a-zA-Z0-9_]+`)

// TestDashboardMetricsExist keeps the committed Grafana dashboard in sync with
// the code: every vigie_* metric the dashboard queries must be exposed by
// /metrics. Without this, a renamed metric silently breaks the dashboard.
func TestDashboardMetricsExist(t *testing.T) {
	// Wire the scrape-time collector and make its gauges emit, so state metrics
	// (vigie_sessions, vigie_db_size_bytes, vigie_watcher_last_seen…) appear.
	srv := newTestServer(t)
	db := filepath.Join(t.TempDir(), "db")
	if err := os.WriteFile(db, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := srv.store.SetMeta(context.Background(), watchSeenKey, "2026-07-31T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	RegisterStateCollector(srv.store, db)

	// A vector metric with no series is absent from the exposition, so drive one
	// series per metric: build info, a valid report (reports/tokens/http), and a
	// rejected report.
	SetBuildInfo("test", "go-test")
	good, _ := json.Marshal(api.ReportRequest{
		Event: "watch", WatcherVersion: version.Version, WatcherCommit: version.Commit, SessionID: "s", Machine: "m", Status: "working",
		Model: "m1", Usage: &api.Usage{OutputTokens: 100}, Timestamp: "2026-07-31T12:00:00Z",
	})
	do(t, srv, http.MethodPost, "/api/report", good, true)
	do(t, srv, http.MethodPost, "/api/report", []byte("{"), true) // bad json → rejected

	raw, err := os.ReadFile("../../dashboards/vigie.json")
	if err != nil {
		t.Fatal(err)
	}
	var dash struct {
		Panels []struct {
			Targets []struct {
				Expr string `json:"expr"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(raw, &dash); err != nil {
		t.Fatalf("dashboard JSON: %v", err)
	}

	wanted := map[string]bool{}
	for _, p := range dash.Panels {
		for _, tgt := range p.Targets {
			for _, m := range metricRe.FindAllString(tgt.Expr, -1) {
				wanted[baseMetric(m)] = true
			}
		}
	}
	if len(wanted) == 0 {
		t.Fatal("no vigie_* metrics referenced by the dashboard — parse error?")
	}

	rec := httptest.NewRecorder()
	MetricsHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	out := rec.Body.String()

	var missing []string
	for m := range wanted {
		if !strings.Contains(out, m) {
			missing = append(missing, m)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("dashboard references metrics not exposed by /metrics: %v", missing)
	}
}

// baseMetric strips a histogram suffix so a query on vigie_x_bucket maps to the
// registered vigie_x (whose HELP/TYPE lines always appear in the exposition).
func baseMetric(name string) string {
	for _, suf := range []string{"_bucket", "_sum", "_count"} {
		name = strings.TrimSuffix(name, suf)
	}
	return name
}
