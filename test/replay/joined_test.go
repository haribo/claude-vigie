// Package replay_test joins the two halves of the test suite.
//
// #817, step 3. The suite already replays chronologies, twice, and neither reaches
// across the seam: `internal/watch/scan_replay_test.go` stops at the report the
// watcher *would* send, and `internal/server/reconcile_timeline_test.go` starts
// from a report written by hand. A defect that lives in what the watcher derives
// from a real artifact, fed into reconciliation, is invisible from both ends —
// which is where #816 lived.
//
// It cannot live in either package. depguard forbids every client-side package
// from linking `internal/server`, as a CI failure rather than a review note
// (ADR-0003), so the only place that may import both is a test-only tree.
package replay_test

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/server"
	"github.com/haribo/claude-vigie/internal/store"
	"github.com/haribo/claude-vigie/internal/watch"
)

const token = "t"

// fleet is the whole chain under test: a real store, a real server, and a HOME the
// registry is written into. Nothing here is a stub — the point of this file is that
// the seam between the two halves is exercised rather than assumed.
type fleet struct {
	t          *testing.T
	srv        *httptest.Server
	home, root string
}

func newFleet(t *testing.T) *fleet {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	st, err := store.Open(filepath.Join(t.TempDir(), "vigie.db"))
	if err != nil {
		t.Fatalf("opening the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	s := server.New(st, token, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	return &fleet{t: t, srv: srv, home: home, root: t.TempDir()}
}

// writeRegistry puts a Claude Code session record on disk, in the shape the
// watcher reads. `waitingFor` carries two lower-case words because that is the
// observed form (#817), not because anything here depends on the words.
func (f *fleet) writeRegistry(sessionID, status, waitingFor string) {
	f.t.Helper()
	dir := filepath.Join(f.home, ".claude", "sessions")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		f.t.Fatal(err)
	}
	start := selfProcStart(f.t)
	rec := fmt.Sprintf(
		`{"sessionId":%q,"status":%q,"waitingFor":%q,"pid":%d,"procStart":%q}`,
		sessionID, status, waitingFor, os.Getpid(), strconv.FormatUint(start, 10))
	p := filepath.Join(dir, strconv.Itoa(os.Getpid())+".json")
	if err := os.WriteFile(p, []byte(rec), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// writeTranscript writes a transcript whose last turn stopped on a tool_use and
// then freezes it at `age` old — the shape a permission prompt leaves behind, and
// the shape a tool the operator has approved leaves behind too. They are
// indistinguishable from here, which is the whole reason the reconciliation rules
// exist.
func (f *fleet) writeTranscript(sessionID string, age time.Duration, now time.Time) {
	f.t.Helper()
	proj := filepath.Join(f.root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		f.t.Fatal(err)
	}
	stamp := now.Add(-age).UTC().Format(time.RFC3339)
	line := fmt.Sprintf(
		`{"sessionId":%q,"cwd":"/work","type":"assistant","timestamp":%q,`+
			`"message":{"id":"m1","stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"just code-check"}}],"usage":{"output_tokens":5}}}`,
		sessionID, stamp)
	p := filepath.Join(proj, sessionID+".jsonl")
	if err := os.WriteFile(p, []byte(line+"\n"), 0o600); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
		f.t.Fatal(err)
	}
}

// scanAndReport runs the *real* watcher over what is on disk and posts what it
// produces to the *real* server. This is the seam: no report is written by hand.
func (f *fleet) scanAndReport(now time.Time) {
	f.t.Helper()
	reports, err := watch.Scan(f.root, "m", 24*time.Hour, now)
	if err != nil {
		f.t.Fatalf("Scan: %v", err)
	}
	if len(reports) == 0 {
		f.t.Fatal("the watcher produced no report; the fixture is not being read")
	}
	for _, r := range reports {
		f.post(r)
	}
}

// hook posts a report the way a Claude Code hook does: an event, and no status —
// the server derives one and stamps it hook-owned.
func (f *fleet) hook(sessionID, event, notificationType string, now time.Time) {
	f.t.Helper()
	f.post(api.ReportRequest{
		SessionID: sessionID, Event: event, Machine: "m",
		NotificationType: notificationType,
		Timestamp:        now.UTC().Format(time.RFC3339),
	})
}

func (f *fleet) post(r api.ReportRequest) {
	f.t.Helper()
	body, _ := json.Marshal(r)
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/api/report", newReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		f.t.Fatalf("posting %s: %v", r.Event, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusMultipleChoices {
		msg, _ := io.ReadAll(resp.Body)
		f.t.Fatalf("posting %s: %d %s", r.Event, resp.StatusCode, msg)
	}
}

// statusOf reads the board the way a client does.
func (f *fleet) statusOf(sessionID string) string {
	f.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, f.srv.URL+"/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		f.t.Fatalf("reading sessions: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var views []api.SessionView
	if err := json.NewDecoder(resp.Body).Decode(&views); err != nil {
		f.t.Fatalf("decoding sessions: %v", err)
	}
	for _, v := range views {
		if v.ID == sessionID {
			return v.Status
		}
	}
	f.t.Fatalf("session %q is not on the board", sessionID)
	return ""
}
