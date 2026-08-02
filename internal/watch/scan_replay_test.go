package watch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
)

// TestScanReplay drives the watcher over faithful transcript fixtures on an
// injected clock (Scan takes now), asserting the status it derives from real
// transcript shapes (stop_reason, content blocks) without a live Claude (#203).
func TestScanReplay(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}

	write := func(id string, ago time.Duration, lines ...string) {
		p := filepath.Join(proj, id+".jsonl")
		if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		ts := now.Add(-ago)
		if err := os.Chtimes(p, ts, ts); err != nil {
			t.Fatal(err)
		}
	}

	// Active turn producing output → working.
	write("act", 2*time.Second,
		`{"type":"assistant","sessionId":"act","message":{"id":"a1","stop_reason":"tool_use","content":[{"type":"text","text":"…"}]}}`)
	// Last content block is a thinking block on a fresh turn → thinking.
	write("think", 2*time.Second,
		`{"type":"assistant","sessionId":"think","message":{"id":"t1","content":[{"type":"thinking","thinking":"…"}]}}`)
	// A live API error → error, carrying the code.
	write("err", 2*time.Second,
		`{"type":"assistant","sessionId":"err","message":{"id":"e1","content":[{"type":"text"}]}}`,
		`{"type":"assistant","sessionId":"err","isApiErrorMessage":true,"apiErrorStatus":529,"message":{}}`)
	// Quiet for a while, no live process mapping → presumed ended.
	write("gone", 20*time.Minute,
		`{"type":"assistant","sessionId":"gone","message":{"id":"g1","stop_reason":"end_turn","content":[{"type":"text"}]}}`)

	reports, err := Scan(root, "m", time.Hour, now)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	byID := map[string]api.ReportRequest{}
	for _, r := range reports {
		byID[r.SessionID] = r
	}

	for id, want := range map[string]string{
		"act": "working", "think": "thinking", "err": "error", "gone": "ended",
	} {
		if got := byID[id].Status; got != want {
			t.Errorf("[%s] derived status = %q, want %q", id, got, want)
		}
	}
	if code := byID["err"].APIErrorStatus; code != 529 {
		t.Errorf("[err] api error code = %d, want 529", code)
	}
}
