package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func TestSettingsRoundTrip(t *testing.T) {
	srv := newTestServer(t)

	// Unset by default.
	rec := do(t, srv, http.MethodGet, "/api/settings", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d", rec.Code)
	}
	var s api.Settings
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s.SessionRetention != "" {
		t.Errorf("default retention = %q, want empty", s.SessionRetention)
	}

	// Set a valid duration, read it back.
	if rec := do(t, srv, http.MethodPost, "/api/settings", []byte(`{"session_retention":"168h"}`), true); rec.Code != http.StatusNoContent {
		t.Fatalf("POST = %d, want 204", rec.Code)
	}
	rec = do(t, srv, http.MethodGet, "/api/settings", nil, true)
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s.SessionRetention != "168h" {
		t.Errorf("retention = %q, want 168h", s.SessionRetention)
	}

	// Invalid duration is rejected.
	if rec := do(t, srv, http.MethodPost, "/api/settings", []byte(`{"session_retention":"nope"}`), true); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid POST = %d, want 400", rec.Code)
	}
}
