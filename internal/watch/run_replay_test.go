package watch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/presence"
)

// replayStart is the loop's logical "now". Every fixture is dated against it, so
// the test never depends on the wall clock — which is the point of #602.
var replayStart = time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

// testClock is the loop's injected wall clock, advanced by the test rather than
// by waiting. The loop reads it from another goroutine, hence the mutex.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// fakeDaemon counts what the loop sent it and decides whether this watcher's
// build is currently acceptable, so a test can put the loop into drift and take
// it back out the way the real daemon does — through the heartbeat's answer.
type fakeDaemon struct {
	mu      sync.Mutex
	refuse  bool
	beats   int
	reports int
}

func (d *fakeDaemon) setRefuse(v bool) {
	d.mu.Lock()
	d.refuse = v
	d.mu.Unlock()
}

func (d *fakeDaemon) counts() (beats, reports int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.beats, d.reports
}

func (d *fakeDaemon) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()
		switch r.URL.Path {
		case "/api/version":
			w.Header().Set("Content-Type", "application/json")
			// version.Version is "dev" in tests; matching it keeps the startup
			// probe quiet so the test is not reading a drift warning it did not ask for.
			_, _ = w.Write([]byte(`{"version":"dev","commit":"none"}`))
		case "/api/watcher/heartbeat":
			d.beats++
			if d.refuse {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"error":"watcher build 0.0.1 does not match this daemon dev"}`))
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "/api/report":
			d.reports++
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
}

// waitFor polls cond until it holds, failing the test if it never does. The loop
// runs in its own goroutine on a real ticker, so the test synchronizes on the
// effects it can observe — requests counted, files written — never on a sleep of
// its own choosing.
func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// holdsFor asserts cond stays true for the whole window — the shape needed for
// "and then nothing happened", which waitFor cannot express.
func holdsFor(t *testing.T, d time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if !cond() {
			t.Fatalf("%s stopped holding", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestRunReplay drives the whole watch loop over an injected clock and a stub
// daemon, asserting the rules that live in the wiring rather than in any one of
// the functions it calls (#602). Each of beat, scan and post is unit-tested
// already; the sequence between them was not covered at all, and docs/code.md
// asks for a replay test over the real path exactly here.
func TestRunReplay(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// One transcript, recent against the injected clock, so every scan has exactly
	// one session to report and `reports` counts scans that actually posted.
	proj := filepath.Join(home, ".claude", "projects", "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	tr := filepath.Join(proj, "sess.jsonl")
	line := `{"type":"assistant","sessionId":"sess","message":{"id":"a1","stop_reason":"tool_use","content":[{"type":"text","text":"…"}]}}` + "\n"
	if err := os.WriteFile(tr, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tr, replayStart.Add(-2*time.Second), replayStart.Add(-2*time.Second)); err != nil {
		t.Fatal(err)
	}

	// A mapping for a process that cannot exist, dated well past MaxAge, so the
	// GC has something to collect when — and only when — the loop decides to run it.
	const deadPID = 1 << 30
	if err := presence.Save("dead", presence.Mapping{PID: deadPID, StartTime: 1}); err != nil {
		t.Fatal(err)
	}
	mapping := filepath.Join(home, ".local", "state", "vigie", "sessions", "dead.json")
	old := replayStart.Add(-2 * time.Hour)
	if err := os.Chtimes(mapping, old, old); err != nil {
		t.Fatal(err)
	}
	mark := filepath.Join(home, ".local", "state", "vigie", "watcher")

	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }

	d := &fakeDaemon{}
	srv := httptest.NewServer(d.handler())
	defer srv.Close()

	clk := &testClock{t: replayStart}
	cfg := &config.Config{ServerURL: srv.URL, Token: "t", Machine: "orion"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// A short real interval only paces the iterations; every cadence the loop
		// decides is read from clk. UsageInterval 0 keeps the usage loop out.
		_ = Run(ctx, cfg, Options{Interval: time.Millisecond, MaxAge: time.Hour, UsageInterval: 0, Now: clk.now})
	}()

	// 1. A healthy watcher beats once, scans, reports, and claims the local mark.
	waitFor(t, func() bool { _, r := d.counts(); return r > 0 }, "the first session report")
	waitFor(t, func() bool { return exists(mark) }, "the local watcher mark")
	if b, _ := d.counts(); b != 1 {
		t.Fatalf("beats = %d after the first scan, want 1", b)
	}

	// 2. The heartbeat is on its own 5 s rhythm, not the scan's: many more scans
	//    must not produce a second beat while the clock has not moved (#386 §3).
	_, r0 := d.counts()
	waitFor(t, func() bool { _, r := d.counts(); return r >= r0+5 }, "five more scans")
	if b, _ := d.counts(); b != 1 {
		t.Fatalf("beats = %d after five more scans on a still clock, want 1", b)
	}

	// 3. A 409 suspends session reports; beats continue, which is what keeps a
	//    refused machine visible instead of vanishing (#384).
	d.setRefuse(true)
	clk.advance(heartbeatInterval)
	waitFor(t, func() bool { b, _ := d.counts(); return b == 2 }, "the refused heartbeat")

	// Let the report count settle before reading it: the loop may have been
	// mid-scan when the refusal landed, and a count taken during that iteration
	// would be compared against a number still moving.
	var atDrift int
	waitFor(t, func() bool {
		_, r := d.counts()
		time.Sleep(5 * time.Millisecond)
		_, again := d.counts()
		atDrift = again
		return r == again
	}, "session reports to stop")

	clk.advance(heartbeatInterval)
	waitFor(t, func() bool { b, _ := d.counts(); return b >= 3 }, "beats to continue while drifted")
	if _, r := d.counts(); r != atDrift {
		t.Errorf("reports = %d while drifted, want %d — a refused watcher must stop reporting sessions", r, atDrift)
	}

	// 4. …and it never claims the local mark while refused: it is not scanning, so
	//    the reporting hooks must keep reading transcripts themselves (#420). The
	//    mark is removed only now, after the refusal has taken effect — removing it
	//    before would race the scans still legitimately running.
	if err := os.Remove(mark); err != nil {
		t.Fatal(err)
	}
	holdsFor(t, 50*time.Millisecond, func() bool { return !exists(mark) }, "the mark staying absent while drifted")

	// 5. A 204 after a 409 resumes reports on its own, with no restart.
	d.setRefuse(false)
	clk.advance(heartbeatInterval)
	waitFor(t, func() bool { _, r := d.counts(); return r > atDrift }, "session reports to resume")
	waitFor(t, func() bool { return exists(mark) }, "the mark to be claimed again")

	// 6. The GC fires on its own interval, read from the same clock: nothing was
	//    collected over all the iterations above, and the dead mapping goes only
	//    once the loop's clock has moved past gcInterval.
	if !exists(mapping) {
		t.Fatal("the dead mapping was collected before gcInterval elapsed")
	}
	clk.advance(gcInterval + time.Second)
	waitFor(t, func() bool { return !exists(mapping) }, "the dead mapping to be collected")

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after its context was canceled")
	}
}
