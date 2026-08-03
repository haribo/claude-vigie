package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
)

const testToken = "test-token"

func newTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st, testToken, nil)
}

func do(t *testing.T, srv *Server, method, path string, body []byte, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if auth {
		r.Header.Set("Authorization", "Bearer "+testToken)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, r)
	return rec
}

func TestHealthHandler(t *testing.T) {
	// /healthz lives on the ops listener now, not the token-protected API mux.
	rec := httptest.NewRecorder()
	newTestServer(t).HealthHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestProtectedRoutesRequireAuth(t *testing.T) {
	srv := newTestServer(t)
	if rec := do(t, srv, http.MethodPost, "/api/report", []byte(`{}`), false); rec.Code != http.StatusUnauthorized {
		t.Errorf("report without auth = %d, want 401", rec.Code)
	}
	if rec := do(t, srv, http.MethodGet, "/api/sessions", nil, false); rec.Code != http.StatusUnauthorized {
		t.Errorf("sessions without auth = %d, want 401", rec.Code)
	}
}

// TestEveryAPIRouteRejectsUnauthenticated is the security invariant: every
// /api/* route must reject a request that carries no token and one that carries
// a wrong token. It guards against a future route registered without s.auth.
// The list is maintained by hand (Go's ServeMux is not introspectable), so a
// newly added protected route must be added here — and an unprotected one will
// fail this test.
func TestEveryAPIRouteRejectsUnauthenticated(t *testing.T) {
	srv := newTestServer(t)
	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/report"},
		{http.MethodGet, "/api/sessions"},
		{http.MethodGet, "/api/events"},
		{http.MethodGet, "/api/usage"},
		{http.MethodPost, "/api/usage"},
		{http.MethodPost, "/api/usage/lease"},
		{http.MethodGet, "/api/watcher"},
		{http.MethodGet, "/api/settings"},
		{http.MethodPost, "/api/settings"},
		{http.MethodGet, "/api/stats"},
	}
	for _, r := range routes {
		// No token at all.
		if rec := do(t, srv, r.method, r.path, []byte(`{}`), false); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token = %d, want 401", r.method, r.path, rec.Code)
		}
		// Present but wrong token (the meaningful proxy for the token check).
		req := httptest.NewRequest(r.method, r.path, bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Authorization", "Bearer wrong-token")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with wrong token = %d, want 401", r.method, r.path, rec.Code)
		}
	}
}

func TestWebDashboardServedUnauthenticated(t *testing.T) {
	srv := newTestServer(t)
	// The app shell and its assets load with no token (they hold no fleet data);
	// the API stays token-protected (covered by TestEveryAPIRouteRejects...).
	for _, path := range []string{"/", "/static/app.js", "/static/app.css"} {
		if rec := do(t, srv, http.MethodGet, path, nil, false); rec.Code != http.StatusOK {
			t.Errorf("GET %s without token = %d, want 200", path, rec.Code)
		}
	}
	// A strict CSP must ride along on the shell.
	rec := do(t, srv, http.MethodGet, "/", nil, false)
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("shell CSP = %q, want default-src 'self'", csp)
	}
	// An unknown path is not the shell — the app is hash-routed, so "/{$}" matches
	// only the exact root and everything else 404s.
	if rec := do(t, srv, http.MethodGet, "/nope", nil, false); rec.Code != http.StatusNotFound {
		t.Errorf("GET /nope = %d, want 404", rec.Code)
	}
}

func TestReportCreatesAndListsSession(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(api.ReportRequest{
		Event:      "UserPromptSubmit",
		SessionID:  "s1",
		Machine:    "laptop",
		ProjectDir: "/p",
		Model:      "claude-opus-4-8",
		Timestamp:  "2026-07-26T10:00:00Z",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code != http.StatusNoContent {
		t.Fatalf("report = %d, want 204 (body: %s)", rec.Code, rec.Body)
	}

	rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("sessions = %d, want 200", rec.Code)
	}
	var views []api.SessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("len = %d, want 1", len(views))
	}
	if views[0].ID != "s1" || views[0].Status != "working" {
		t.Errorf("got id=%s status=%s, want s1/working", views[0].ID, views[0].Status)
	}
	if views[0].StartedAt != "2026-07-26T10:00:00Z" {
		t.Errorf("started_at = %q", views[0].StartedAt)
	}
}

func TestReportWithoutUsagePreservesTokens(t *testing.T) {
	srv := newTestServer(t)

	// First: Stop carrying usage.
	stop, _ := json.Marshal(api.ReportRequest{
		Event: "Stop", SessionID: "s1", Machine: "m", ProjectDir: "/p",
		Timestamp: "2026-07-26T10:00:00Z",
		Usage:     &api.Usage{InputTokens: 100, OutputTokens: 50},
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", stop, true); rec.Code != http.StatusNoContent {
		t.Fatalf("first report = %d", rec.Code)
	}

	// Second: a prompt submit with no usage must not zero the tokens.
	prompt, _ := json.Marshal(api.ReportRequest{
		Event: "UserPromptSubmit", SessionID: "s1", Machine: "m", ProjectDir: "/p",
		Timestamp: "2026-07-26T10:01:00Z",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", prompt, true); rec.Code != http.StatusNoContent {
		t.Fatalf("second report = %d", rec.Code)
	}

	rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
	var views []api.SessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("len = %d, want 1", len(views))
	}
	if views[0].Usage.InputTokens != 100 || views[0].Usage.OutputTokens != 50 {
		t.Errorf("usage not preserved: %+v", views[0].Usage)
	}
	if views[0].Status != "working" {
		t.Errorf("status = %q, want working", views[0].Status)
	}
}

func TestReportPreservesContext(t *testing.T) {
	srv := newTestServer(t)

	start, _ := json.Marshal(api.ReportRequest{
		Event: "SessionStart", SessionID: "s1", Machine: "laptop", ProjectDir: "/p",
		GitBranch: "feature-x", Model: "claude-opus-4-8", Timestamp: "2026-07-26T10:00:00Z",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", start, true); rec.Code != http.StatusNoContent {
		t.Fatalf("start = %d", rec.Code)
	}

	// A later event omits git_branch and model; they must be preserved.
	stop, _ := json.Marshal(api.ReportRequest{
		Event: "Stop", SessionID: "s1", Machine: "laptop", ProjectDir: "/p",
		Timestamp: "2026-07-26T10:05:00Z",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", stop, true); rec.Code != http.StatusNoContent {
		t.Fatalf("stop = %d", rec.Code)
	}

	rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
	var views []api.SessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("len = %d, want 1", len(views))
	}
	if views[0].GitBranch != "feature-x" {
		t.Errorf("git_branch = %q, want feature-x (preserved)", views[0].GitBranch)
	}
	if views[0].Model != "claude-opus-4-8" {
		t.Errorf("model = %q, want preserved", views[0].Model)
	}
}

func TestReportExplicitStatusWins(t *testing.T) {
	srv := newTestServer(t)

	// A watcher report carries an explicit status and must not append an event.
	body, _ := json.Marshal(api.ReportRequest{
		Event: "watch", SessionID: "s1", Machine: "m", ProjectDir: "/p",
		Status: "waiting", Timestamp: "2026-07-26T10:00:00Z",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code != http.StatusNoContent {
		t.Fatalf("report = %d", rec.Code)
	}

	rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
	var views []api.SessionView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Status != "waiting" {
		t.Fatalf("status = %+v, want explicit 'waiting'", views)
	}
}

func TestRemoteURLSurfacedAndCleared(t *testing.T) {
	srv := newTestServer(t)
	on, off := true, false

	// A watch report with remote control active carries the resume URL.
	body, _ := json.Marshal(api.ReportRequest{
		Event: "watch", SessionID: "s1", Machine: "m", ProjectDir: "/p",
		Status: "working", RemoteControl: &on, RemoteURL: "https://claude.ai/code/session_01AB",
		Timestamp: "2026-07-26T10:00:00Z",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code != http.StatusNoContent {
		t.Fatalf("report = %d", rec.Code)
	}
	viewOf := func() api.SessionView {
		rec := do(t, srv, http.MethodGet, "/api/sessions", nil, true)
		var views []api.SessionView
		if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil || len(views) != 1 {
			t.Fatalf("sessions = %s (err %v)", rec.Body, err)
		}
		return views[0]
	}
	if v := viewOf(); !v.RemoteControl || v.RemoteURL != "https://claude.ai/code/session_01AB" {
		t.Fatalf("remote not surfaced: rc=%v url=%q", v.RemoteControl, v.RemoteURL)
	}

	// When /rc is switched off the URL is cleared with the flag.
	off2, _ := json.Marshal(api.ReportRequest{
		Event: "watch", SessionID: "s1", Machine: "m", ProjectDir: "/p",
		Status: "working", RemoteControl: &off, RemoteURL: "", Timestamp: "2026-07-26T10:01:00Z",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", off2, true); rec.Code != http.StatusNoContent {
		t.Fatalf("report off = %d", rec.Code)
	}
	if v := viewOf(); v.RemoteControl || v.RemoteURL != "" {
		t.Errorf("remote not cleared: rc=%v url=%q", v.RemoteControl, v.RemoteURL)
	}
}

func TestWatcherStatusPerMachine(t *testing.T) {
	srv := newTestServer(t)

	// alpha reports through the watcher; beta has a session but reports on hooks alone.
	watch, _ := json.Marshal(api.ReportRequest{
		Event: "watch", SessionID: "a1", Machine: "alpha", ProjectDir: "/p",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", watch, true); rec.Code != http.StatusNoContent {
		t.Fatalf("alpha watch report = %d", rec.Code)
	}
	hook, _ := json.Marshal(api.ReportRequest{
		Event: "UserPromptSubmit", SessionID: "b1", Machine: "beta", ProjectDir: "/p",
	})
	if rec := do(t, srv, http.MethodPost, "/api/report", hook, true); rec.Code != http.StatusNoContent {
		t.Fatalf("beta hook report = %d", rec.Code)
	}

	rec := do(t, srv, http.MethodGet, "/api/watcher", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("watcher = %d, want 200", rec.Code)
	}
	var ws api.WatcherStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &ws); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ws.Machines["alpha"] == "" {
		t.Errorf("alpha should carry a watcher heartbeat, got %+v", ws.Machines)
	}
	if v, ok := ws.Machines["beta"]; !ok || v != "" {
		t.Errorf("beta should be present with an empty heartbeat, got %q (ok=%v)", v, ok)
	}
}

func TestReportValidation(t *testing.T) {
	srv := newTestServer(t)
	if rec := do(t, srv, http.MethodPost, "/api/report", []byte(`{"event":"Stop"}`), true); rec.Code != http.StatusBadRequest {
		t.Errorf("missing session_id = %d, want 400", rec.Code)
	}
	if rec := do(t, srv, http.MethodPost, "/api/report", []byte(`not json`), true); rec.Code != http.StatusBadRequest {
		t.Errorf("bad json = %d, want 400", rec.Code)
	}
}

func TestSSEDeltaGating(t *testing.T) {
	srv := newTestServer(t)
	ch := srv.hub.subscribe()
	published := func() bool { // non-blocking: did a report fan out an SSE event?
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}
	report := func(status, ts string, out int64) {
		t.Helper()
		body, _ := json.Marshal(api.ReportRequest{
			Event: "watch", SessionID: "s1", Machine: "m", ProjectDir: "/p",
			Status: status, Usage: &api.Usage{OutputTokens: out}, Timestamp: ts,
		})
		if rec := do(t, srv, http.MethodPost, "/api/report", body, true); rec.Code != http.StatusNoContent {
			t.Fatalf("report = %d", rec.Code)
		}
	}

	report("working", "2026-08-02T10:00:00Z", 100)
	if !published() {
		t.Fatal("a new session must publish")
	}
	// Same visible state, later timestamp (heartbeat only) → no publish.
	report("working", "2026-08-02T10:00:02Z", 100)
	if published() {
		t.Error("an unchanged report must not publish (only the heartbeat moved)")
	}
	// Real change (status) → publish.
	report("waiting", "2026-08-02T10:00:04Z", 100)
	if !published() {
		t.Error("a status change must publish")
	}
	// Real change (tokens grew) → publish.
	report("waiting", "2026-08-02T10:00:06Z", 250)
	if !published() {
		t.Error("a token change must publish")
	}
}
