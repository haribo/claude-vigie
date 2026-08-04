package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
)

func TestToViewAPIError(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-2 * time.Second).Format(time.RFC3339)
	v := toView(store.Session{Status: "error", APIErrorStatus: 529, ReportedAt: fresh}, nil, now, true)
	if v.Status != "error" || v.APIErrorStatus != 529 {
		t.Errorf("view = {%q, %d}, want {error, 529}", v.Status, v.APIErrorStatus)
	}
}

// A watch report carrying an API error round-trips through the store to the view.
func TestReportAPIErrorFlow(t *testing.T) {
	srv := newTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	body, _ := json.Marshal(api.ReportRequest{
		Event: "watch", SessionID: "s-e", Machine: "m",
		Status: "error", APIErrorStatus: 529, Timestamp: now,
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code >= http.StatusMultipleChoices {
		t.Fatalf("post report = %d", rec.Code)
	}

	rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("get sessions = %d", rec.Code)
	}
	var views []api.SessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Status != "error" || views[0].APIErrorStatus != 529 {
		t.Fatalf("views = %+v, want one error/529", views)
	}
}
