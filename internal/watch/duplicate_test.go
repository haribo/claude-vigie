package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// One session, two transcript files — #432, observed in production.
//
// Claude Code stores transcripts under `~/.claude/projects/<encoded-cwd>/`, so a
// session whose working directory changes is written under two directories with
// the same session id. The scanner globbed `*/*.jsonl` and emitted one report per
// *file*, so each scan reported the live total and then the abandoned file's,
// which was zero. Downstream that read as the whole total being produced afresh,
// every scan, for half a day.
func TestScanReportsOneReportPerSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()

	// The live transcript, under the project's current path.
	live := filepath.Join(root, "-home-u-plain-note")
	if err := os.MkdirAll(live, 0o750); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(live, "dup.jsonl"), []string{
		`{"sessionId":"dup","cwd":"/home/u/plain-note","type":"assistant","message":{"id":"m1","model":"claude-opus-4-8","stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":2713408}}}`,
	})

	// The stub left behind under the old path: metadata only, not one exchange.
	// This is what the real file contained — nine lines, no conversation.
	stale := filepath.Join(root, "-home-u-note")
	if err := os.MkdirAll(stale, 0o750); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(stale, "dup.jsonl"), []string{
		`{"type":"custom-title","customTitle":"plain-note","sessionId":"dup"}`,
		`{"type":"permission-mode","permissionMode":"auto","sessionId":"dup"}`,
	})

	reports, err := newScanner().scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	var seen int
	for _, r := range reports {
		if r.SessionID == "dup" {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("session reported %d times in one scan, want 1 — two files, one session", seen)
	}

	got := reportFor(reports, "dup")
	if got == nil {
		t.Fatal("the session disappeared entirely")
	}
	if got.Usage == nil || got.Usage.OutputTokens != 2713408 {
		t.Errorf("usage = %+v, want the live transcript's 2713408", got.Usage)
	}
	if got.Model != "claude-opus-4-8" {
		t.Errorf("model = %q, want the live transcript's model", got.Model)
	}
}

// TestScanKeepsTheLargestTotalWhicheverOrder: the glob returns paths sorted, so
// the abandoned file can come first. The winner must be chosen by the totals, not
// by the order the files happen to be read in.
func TestScanKeepsTheLargestTotalWhicheverOrder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	for _, order := range []struct{ first, second string }{
		{"a-old", "z-new"},
		{"z-old", "a-new"},
	} {
		root := t.TempDir()
		for _, d := range []struct {
			dir    string
			tokens string
		}{{order.first, "10"}, {order.second, "999999"}} {
			p := filepath.Join(root, d.dir)
			if err := os.MkdirAll(p, 0o750); err != nil {
				t.Fatal(err)
			}
			writeLines(t, filepath.Join(p, "dup.jsonl"), []string{
				`{"sessionId":"dup","cwd":"/w","type":"assistant","message":{"id":"m` + d.tokens +
					`","model":"claude-opus-4-8","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":` + d.tokens + `}}}`,
			})
		}

		reports, err := newScanner().scan(root, "m", 24*time.Hour, time.Now())
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		got := reportFor(reports, "dup")
		if got == nil || got.Usage == nil || got.Usage.OutputTokens != 999999 {
			t.Errorf("dirs %s/%s: usage = %+v, want 999999", order.first, order.second, got.Usage)
		}
	}
}

// TestScanLeavesDistinctSessionsAlone guards the fix from collapsing real
// sessions: de-duplication is by session id, nothing else.
func TestScanLeavesDistinctSessionsAlone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two", "three"} {
		writeLines(t, filepath.Join(proj, id+".jsonl"), []string{
			`{"sessionId":"` + id + `","cwd":"/w","type":"assistant","message":{"id":"m","model":"claude-opus-4-8","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":7}}}`,
		})
	}

	reports, err := newScanner().scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(reports) != 3 {
		t.Errorf("got %d reports for three distinct sessions", len(reports))
	}
}
