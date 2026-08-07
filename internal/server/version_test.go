package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// TestGetVersion covers the #341 daemon build endpoint: authenticated, returns a
// non-empty version (a dev build reads "dev"), and rejects a missing token.
func TestGetVersion(t *testing.T) {
	srv := newTestServer(t)

	rec := do(t, srv, http.MethodGet, "/api/version", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("get version = %d", rec.Code)
	}
	var v api.VersionInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Version == "" {
		t.Error("version must never be empty — a dev build reads \"dev\"")
	}

	if rec := do(t, srv, http.MethodGet, "/api/version", nil, false); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated version = %d, want 401", rec.Code)
	}
}
