package server

import (
	"encoding/json"
	"net/http"
	"strings"
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

// #558. `session_retention` decides what the prune loop deletes, and it accepted
// any duration Go can parse. `1ns` is one: the next pass computes `now - 1ns` as
// its cutoff and removes every session, event and token sample — live ones
// included, since the predicate is last-report time and not status.
//
// The floor is not a security control. Anyone who can set this holds the fleet
// token, and a token holder can already make the board lie in other ways. It
// guards a *mistake*: Go durations have no month unit, so an operator who wants
// thirty days and types `30s` instead of `720h` deletes their history by hand,
// and nothing between the keystroke and the deletion says otherwise.
//
// One hour is far below the smallest window the TUI offers (24 h, `retentionPresets`
// in internal/tui/prefs.go) and far above anything that could be meant seriously.
func TestRetentionShorterThanAnHourIsRefused(t *testing.T) {
	srv := newTestServer(t)
	for _, d := range []string{"1ns", "1s", "30s", "59m", "0s"} {
		rec := do(t, srv, http.MethodPost, "/api/settings", []byte(`{"session_retention":"`+d+`"}`), true)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST /api/settings %s = %d, want 400 — this empties the database on the next prune", d, rec.Code)
		}
	}
}

func TestTheRetentionAnOperatorWouldActuallySetIsAccepted(t *testing.T) {
	srv := newTestServer(t)
	// "" is "keep forever" and must stay valid — it is how pruning is disabled.
	for _, d := range []string{"", "1h", "24h", "168h", "720h"} {
		rec := do(t, srv, http.MethodPost, "/api/settings", []byte(`{"session_retention":"`+d+`"}`), true)
		if rec.Code != http.StatusNoContent {
			t.Errorf("POST /api/settings %q = %d, want 204", d, rec.Code)
		}
	}
}

// A refused value must leave the stored one alone: a rejected write that still
// half-applied would be worse than the value it refused.
func TestARefusedRetentionDoesNotOverwriteTheStoredOne(t *testing.T) {
	srv := newTestServer(t)
	if rec := do(t, srv, http.MethodPost, "/api/settings", []byte(`{"session_retention":"168h"}`), true); rec.Code != http.StatusNoContent {
		t.Fatalf("setting a good value = %d", rec.Code)
	}
	if rec := do(t, srv, http.MethodPost, "/api/settings", []byte(`{"session_retention":"1ns"}`), true); rec.Code != http.StatusBadRequest {
		t.Fatalf("setting 1ns = %d, want 400", rec.Code)
	}
	rec := do(t, srv, http.MethodGet, "/api/settings", nil, true)
	if !strings.Contains(rec.Body.String(), "168h") {
		t.Errorf("the stored retention is %s — the refused write changed it", rec.Body.String())
	}
}
