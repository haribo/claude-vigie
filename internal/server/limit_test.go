package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestReportBodyTooLarge(t *testing.T) {
	srv := newTestServer(t)
	// Valid JSON, but a field larger than the cap, so the decoder reads past the
	// limit and gets an *http.MaxBytesError (an invalid body would fail earlier).
	big := []byte(`{"session_id":"s","event":"watch","machine":"` +
		strings.Repeat("a", maxBodyBytes) + `"}`)
	if rec := do(t, srv, http.MethodPost, "/api/report", big, true); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: status = %d, want 413", rec.Code)
	}

	// A normal body still succeeds.
	ok := []byte(`{"session_id":"s","event":"watch",` + watchBuildJSON() +
		`"machine":"m","status":"working","timestamp":"2026-07-31T12:00:00Z"}`)
	if rec := do(t, srv, http.MethodPost, "/api/report", ok, true); rec.Code >= http.StatusMultipleChoices {
		t.Fatalf("normal body: status = %d", rec.Code)
	}
}
