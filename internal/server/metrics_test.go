package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestMetricsExposition(t *testing.T) {
	srv := newTestServer(t)
	SetBuildInfo("test", "go-test")
	RegisterStateCollector(srv.store, "no-such.db") // db_size just skips when stat fails

	// Ingest a report so the counters and the state gauge move.
	body, _ := json.Marshal(api.ReportRequest{
		Event: "watch", SessionID: "s", Machine: "m", Status: "working",
		Timestamp: "2026-07-31T12:00:00Z",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code >= http.StatusMultipleChoices {
		t.Fatalf("report = %d", rec.Code)
	}

	rec := httptest.NewRecorder()
	MetricsHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics = %d", rec.Code)
	}
	out := rec.Body.String()
	for _, want := range []string{
		`vigie_reports_total{event="watch"}`,
		`vigie_http_requests_total`,
		`vigie_http_request_duration_seconds`,
		`vigie_sessions{status="working"}`,
		`vigie_build_info{`,
		`vigie_sse_events_published_total`,
		`go_goroutines`, // default Go collector is registered
	} {
		if !strings.Contains(out, want) {
			t.Errorf("/metrics missing %q", want)
		}
	}
}
