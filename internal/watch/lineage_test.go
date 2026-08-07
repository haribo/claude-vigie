package watch

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/presence"
)

// reportFor returns the report for a session id, or nil.
func reportFor(reports []api.ReportRequest, id string) *api.ReportRequest {
	for i := range reports {
		if reports[i].SessionID == id {
			return &reports[i]
		}
	}
	return nil
}

// liveRegistry writes a registry record for a session on this test process (so
// its liveness check passes), status busy.
func liveRegistry(t *testing.T, home, file, sessionID string, pid int, ps uint64) {
	t.Helper()
	writeSession(t, home, file,
		`{"sessionId":"`+sessionID+`","status":"busy","pid":`+strconv.Itoa(pid)+
			`,"procStart":"`+strconv.FormatUint(ps, 10)+`"}`)
}

// TestScanCarriesModelEffortAcrossClear proves a /clear'd session inherits the
// process's last known model and effort instead of showing "-" until its first
// turn (#367, point B). The old session ran turns (real model/effort); the same
// process then switches to a fresh session id whose transcript has no assistant
// line yet.
func TestScanCarriesModelEffortAcrossClear(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pid, ps := os.Getpid(), selfStartTime(t)

	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}

	// Scan 1: the original session, live, with a real model and effort.
	liveRegistry(t, home, "p.json", "old", pid, ps)
	writeLines(t, filepath.Join(proj, "old.jsonl"), []string{
		`{"sessionId":"old","cwd":"/work","type":"assistant","effort":"high","message":{"id":"m1","model":"claude-opus-4-8","stop_reason":"end_turn","usage":{"input_tokens":1000,"output_tokens":5}}}`,
	})

	sc := newScanner()
	r1, err := sc.scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("scan 1: %v", err)
	}
	if got := reportFor(r1, "old"); got == nil || got.Model != "claude-opus-4-8" || got.Effort != "high" {
		t.Fatalf("scan 1 old report = %+v, want model/effort populated", got)
	}

	// /clear: the same process now backs a fresh session id, whose transcript has
	// only a user line — no assistant line, so no model/effort of its own yet.
	liveRegistry(t, home, "p.json", "new", pid, ps)
	writeLines(t, filepath.Join(proj, "new.jsonl"), []string{
		`{"sessionId":"new","cwd":"/work","type":"user","message":{"role":"user","content":"hello"}}`,
	})

	r2, err := sc.scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("scan 2: %v", err)
	}
	got := reportFor(r2, "new")
	if got == nil {
		t.Fatal("scan 2: no report for the cleared session")
	}
	if got.Model != "claude-opus-4-8" {
		t.Errorf("cleared session model = %q, want carried-over claude-opus-4-8 (#367)", got.Model)
	}
	if got.Effort != "high" {
		t.Errorf("cleared session effort = %q, want carried-over high (#367)", got.Effort)
	}
}

// TestScanCarryOverIsPerProcess proves the carry-over never leaks across
// processes: a fresh session on a *different* process does not inherit another
// process's model/effort (#367, point B).
func TestScanCarryOverIsPerProcess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pid, ps := os.Getpid(), selfStartTime(t)

	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}

	// Process A ran a session with a real model.
	liveRegistry(t, home, "a.json", "a-old", pid, ps)
	writeLines(t, filepath.Join(proj, "a-old.jsonl"), []string{
		`{"sessionId":"a-old","cwd":"/work","type":"assistant","effort":"high","message":{"id":"m1","model":"claude-opus-4-8","stop_reason":"end_turn","usage":{"input_tokens":1000,"output_tokens":5}}}`,
	})

	sc := newScanner()
	if _, err := sc.scan(root, "m", 24*time.Hour, time.Now()); err != nil {
		t.Fatalf("scan 1: %v", err)
	}

	// A different process (distinct procStart) starts a fresh session with no
	// assistant line. It must NOT inherit process A's model/effort.
	liveRegistry(t, home, "b.json", "b-new", pid, ps+1)
	writeLines(t, filepath.Join(proj, "b-new.jsonl"), []string{
		`{"sessionId":"b-new","cwd":"/work","type":"user","message":{"role":"user","content":"hi"}}`,
	})

	r2, err := sc.scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("scan 2: %v", err)
	}
	got := reportFor(r2, "b-new")
	if got == nil {
		t.Fatal("no report for b-new")
	}
	if got.Model != "" || got.Effort != "" {
		t.Errorf("b-new inherited model=%q effort=%q from another process; want empty (#367)", got.Model, got.Effort)
	}
}

// TestScanSupersededSessionIsEnded proves the pre-clear ghost row is retired: a
// session that left the registry but whose (reused) process now runs a different
// session id reports ended, not a lingering idle (#367, point C).
func TestScanSupersededSessionIsEnded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pid, ps := os.Getpid(), selfStartTime(t)

	// The live process now runs the fresh session "new".
	liveRegistry(t, home, "p.json", "new", pid, ps)
	// The old session's presence mapping still points at that same live process.
	if err := presence.Save("old", presence.Mapping{PID: pid, StartTime: ps}); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	// The old transcript is still fresh; without the supersede rule its alive
	// (reused) process would make it read idle.
	writeLines(t, filepath.Join(proj, "old.jsonl"), []string{
		`{"sessionId":"old","cwd":"/work","timestamp":"2026-01-01T00:00:00Z","type":"assistant","message":{"id":"m","stop_reason":"end_turn","usage":{"output_tokens":1}}}`,
	})

	reports, err := Scan(root, "m", 100000*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got := reportFor(reports, "old")
	if got == nil {
		t.Fatal("no report for the superseded session")
	}
	if got.Status != "ended" {
		t.Errorf("superseded session status = %q, want ended (#367)", got.Status)
	}
}

// TestScanReportsKnownContext proves the watcher always reports a *known* context
// (a non-nil pointer), including a known 0 for a fresh transcript with no usage —
// so the TUI shows 0%, not "-" (#367, point A).
func TestScanReportsKnownContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pid, ps := os.Getpid(), selfStartTime(t)
	liveRegistry(t, home, "p.json", "fresh", pid, ps)

	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	// A just-cleared transcript: a user line, no assistant usage → 0 context.
	writeLines(t, filepath.Join(proj, "fresh.jsonl"), []string{
		`{"sessionId":"fresh","cwd":"/work","type":"user","message":{"role":"user","content":"hi"}}`,
	})

	reports, err := Scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got := reportFor(reports, "fresh")
	if got == nil {
		t.Fatal("no report for fresh")
	}
	if got.ContextTokens == nil {
		t.Fatalf("fresh session context is nil (unknown); want a known 0 so the TUI shows 0%% (#367)")
	}
	if *got.ContextTokens != 0 {
		t.Errorf("fresh session context = %d, want 0", *got.ContextTokens)
	}
}
