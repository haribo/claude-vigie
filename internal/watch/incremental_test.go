package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestScannerResumesOnGrowth checks the incremental path (#257): after a scan, a
// transcript that grows is resumed from the cached offset, so accumulated usage
// reflects the appended lines without a full re-parse.
func TestScannerResumesOnGrowth(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no session registry → transcript-only derivation
	root := t.TempDir()
	proj := filepath.Join(root, "p")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(proj, "s1.jsonl")
	writeLines(t, f, []string{
		`{"sessionId":"s1","type":"assistant","message":{"id":"m1","stop_reason":"end_turn","usage":{"output_tokens":50}}}`,
	})

	sc := newScanner()
	out := func() int64 {
		t.Helper()
		reports, err := sc.scan(root, "m", 24*time.Hour, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range reports {
			if r.SessionID == "s1" {
				return r.Usage.OutputTokens
			}
		}
		t.Fatal("no report for s1")
		return 0
	}
	if got := out(); got != 50 {
		t.Fatalf("first scan output = %d, want 50", got)
	}

	// Append a second assistant message (the file grows; mtime moves).
	af, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := af.WriteString(`{"sessionId":"s1","type":"assistant","message":{"id":"m2","stop_reason":"end_turn","usage":{"output_tokens":30}}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = af.Close()

	if got := out(); got != 80 { // 50 + 30, accumulated across the resumed parse
		t.Errorf("after append, output = %d, want 80 (resumed)", got)
	}
}
