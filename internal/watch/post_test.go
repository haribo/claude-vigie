package watch

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/config"
)

func TestPostJSON(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody api.ReportRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if r.URL.Path == "/api/usage" {
			_ = json.NewEncoder(w).Encode(api.UsageReport{FiveHourPct: 42})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	cfg := &config.Config{ServerURL: srv.URL, Token: "tok"}

	// POSTs the body with the bearer token; no response to decode.
	if err := postJSON(cfg, "/api/report", api.ReportRequest{SessionID: "s1", Event: "watch"}, nil); err != nil {
		t.Fatalf("postJSON: %v", err)
	}
	if gotAuth != "Bearer tok" || gotPath != "/api/report" || gotBody.SessionID != "s1" {
		t.Errorf("server saw auth=%q path=%q body=%+v", gotAuth, gotPath, gotBody)
	}

	// Decodes the response into out.
	var out api.UsageReport
	if err := postJSON(cfg, "/api/usage", api.LeaseRequest{Holder: "h"}, &out); err != nil {
		t.Fatalf("postJSON decode: %v", err)
	}
	if out.FiveHourPct != 42 {
		t.Errorf("decoded = %+v, want FiveHourPct 42", out)
	}

	// A non-2xx status is an error.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer bad.Close()
	if err := postJSON(&config.Config{ServerURL: bad.URL, Token: "x"}, "/api/report", api.ReportRequest{}, nil); err == nil {
		t.Error("expected an error on HTTP 401, got nil")
	}
}
