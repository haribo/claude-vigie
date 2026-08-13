package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func postReport(t *testing.T, srv *Server, body string) {
	t.Helper()
	if rec := do(t, srv, http.MethodPost, "/api/report", []byte(body), true); rec.Code != http.StatusNoContent {
		t.Fatalf("report %s = %d, want 204", body, rec.Code)
	}
}

func TestStatsRollupAndEndpoint(t *testing.T) {
	srv := newTestServer(t)

	// Watch reports accrue output-token deltas (500, then +300 = 800).
	postReport(t, srv, `{"event":"watch",`+watchBuildJSON()+`"session_id":"s1","machine":"m","model":"opus","status":"working","usage":{"output_tokens":500},"timestamp":"2026-07-29T10:00:00Z"}`)
	postReport(t, srv, `{"event":"watch",`+watchBuildJSON()+`"session_id":"s1","machine":"m","model":"opus","status":"working","usage":{"output_tokens":800},"timestamp":"2026-07-29T10:01:00Z"}`)

	// Hook events accrue status seconds: working 10:02→10:04 (120s), then
	// waiting 10:04→10:07 (180s).
	postReport(t, srv, `{"event":"UserPromptSubmit","session_id":"s1","machine":"m","model":"opus","timestamp":"2026-07-29T10:02:00Z"}`)
	postReport(t, srv, `{"event":"Notification","session_id":"s1","machine":"m","model":"opus","timestamp":"2026-07-29T10:04:00Z"}`)
	postReport(t, srv, `{"event":"Stop","session_id":"s1","machine":"m","model":"opus","timestamp":"2026-07-29T10:07:00Z"}`)

	rec := do(t, srv, http.MethodGet, "/api/stats", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats = %d, want 200", rec.Code)
	}
	var resp api.StatsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if resp.SessionCount != 1 {
		t.Errorf("session_count = %d, want 1", resp.SessionCount)
	}
	if len(resp.Daily) != 1 {
		t.Fatalf("daily rows = %d, want 1", len(resp.Daily))
	}
	d := resp.Daily[0]
	if d.Day != "2026-07-29" || d.Model != "opus" {
		t.Errorf("daily key = %s/%s, want 2026-07-29/opus", d.Day, d.Model)
	}
	if d.OutputTokens != 800 {
		t.Errorf("output_tokens = %d, want 800", d.OutputTokens)
	}
	if d.WorkingSeconds != 120 {
		t.Errorf("working_seconds = %d, want 120", d.WorkingSeconds)
	}
	if d.WaitingSeconds != 180 {
		t.Errorf("waiting_seconds = %d, want 180", d.WaitingSeconds)
	}

	if len(resp.TopSessions) != 1 || resp.TopSessions[0].Name != "s1" {
		t.Fatalf("top_sessions = %+v, want one named s1", resp.TopSessions)
	}
	if resp.TopSessions[0].OutputTokens != 800 {
		t.Errorf("top session tokens = %d, want 800", resp.TopSessions[0].OutputTokens)
	}
}
