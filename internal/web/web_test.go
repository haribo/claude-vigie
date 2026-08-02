package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	// The daemon mux strips nothing before this handler, so the request path is
	// the full "/" or "/static/...".
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestServesAppShell(t *testing.T) {
	rec := get(t, Handler(), "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>claude-vigie</title>") {
		t.Errorf("GET / did not serve the app shell:\n%s", rec.Body.String()[:min(200, rec.Body.Len())])
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := get(t, Handler(), "/")
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP = %q, want a same-origin default-src", csp)
	}
	if strings.Contains(csp, "unsafe-inline") {
		t.Errorf("CSP allows unsafe-inline: %q", csp)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options: nosniff")
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("shell Cache-Control = %q, want no-cache", rec.Header().Get("Cache-Control"))
	}
}

func TestServesStaticAssets(t *testing.T) {
	for _, tc := range []struct{ path, ctPrefix, needle string }{
		{"/static/app.js", "text/javascript", "claude-vigie"},
		{"/static/app.css", "text/css", "--accent"},
	} {
		rec := get(t, Handler(), tc.path)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", tc.path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tc.ctPrefix) {
			t.Errorf("%s Content-Type = %q, want %s", tc.path, ct, tc.ctPrefix)
		}
		if !strings.Contains(rec.Body.String(), tc.needle) {
			t.Errorf("%s missing expected content %q", tc.path, tc.needle)
		}
		if rec.Header().Get("Content-Security-Policy") == "" {
			t.Errorf("%s missing CSP header", tc.path)
		}
	}
}
