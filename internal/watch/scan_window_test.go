package watch

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// #660. The scan skips a transcript older than `--max-age` before it ever
// consults Claude Code's registry, so a session the registry lists as live —
// with its pid, which is proof the process is running — is never reported. The
// server then reads that silence on a watched machine as `ended`
// (internal/server/sessions.go), and a session open on the operator's screen is
// announced as over.
//
// The window has a real job: there are hundreds of transcripts on a working
// machine and it keeps the scan from re-reading their history every couple of
// seconds. What it must not do is hide a session whose process is alive. Bounding
// the exception by the registry keeps the cost bounded by live Claude processes
// rather than by disk history.
//
// These use a realistic window on purpose. Every other scan test passes
// `100000*time.Hour`, which is why the window's effect was never exercised.

// oldTranscript writes a transcript and backdates it past any scan window.
func oldTranscript(t *testing.T, proj, id string) {
	t.Helper()
	p := filepath.Join(proj, id+".jsonl")
	writeLines(t, p, []string{
		`{"sessionId":"` + id + `","timestamp":"2026-01-01T00:00:00Z","type":"assistant","message":{"id":"m","stop_reason":"end_turn","usage":{"output_tokens":1}}}`,
	})
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
}

func scanRoot(t *testing.T) (root, proj string) {
	t.Helper()
	root = t.TempDir()
	proj = filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	return root, proj
}

// A session the registry lists is alive, however long its transcript has been
// quiet. It must be reported, and it must not read `ended`.
func TestALiveSessionOutlivesTheScanWindow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ps := selfProcStart(t)
	writeSession(t, home, "live.json",
		`{"sessionId":"s-live","status":"idle","pid":`+strconv.Itoa(os.Getpid())+`,"procStart":"`+strconv.FormatUint(ps, 10)+`"}`)

	root, proj := scanRoot(t)
	oldTranscript(t, proj, "s-live")

	reports, err := Scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, r := range reports {
		if r.SessionID == "s-live" {
			if r.Status == "ended" {
				t.Errorf("status = %q for a session whose process is running", r.Status)
			}
			return
		}
	}
	t.Fatal("no report for a live session whose transcript is older than the window — the server reads that silence as ended")
}

// The window still does its job for everything else: an old transcript whose
// session Claude Code no longer lists stays out of the scan. Without this the
// exception would put every transcript ever written back on the hot path.
func TestAnOldTranscriptOutsideTheRegistryIsStillSkipped(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // an empty registry
	root, proj := scanRoot(t)
	oldTranscript(t, proj, "s-gone")

	reports, err := Scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, r := range reports {
		if r.SessionID == "s-gone" {
			t.Fatal("an old transcript nothing lists was scanned; the window bounds nothing")
		}
	}
}

// The other half of the rule: a session stays open while a process runs, and ends
// when one does not. A registry record whose process is gone reads `ended` even
// with a transcript far outside the window — the case that must keep working once
// the window stops hiding live sessions.
func TestARegisteredSessionWithADeadProcessIsEnded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSession(t, home, "dead.json",
		`{"sessionId":"s-dead","status":"busy","pid":1073741824,"procStart":"1"}`)

	root, proj := scanRoot(t)
	oldTranscript(t, proj, "s-dead")

	reports, err := Scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, r := range reports {
		if r.SessionID == "s-dead" {
			if r.Status != "ended" {
				t.Errorf("status = %q, want ended (the backing process is gone)", r.Status)
			}
			return
		}
	}
	t.Fatal("no report for a registered session whose process is gone")
}
