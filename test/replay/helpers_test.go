package replay_test

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

func newReader(b []byte) io.Reader { return bytes.NewReader(b) }

// selfProcStart reads this process's start time from /proc, so a fixture can
// claim a pid that is genuinely alive. Without it the watcher reads the session's
// backing process as gone and reports `ended` before any reconciliation rule is
// consulted, and the test would pin nothing.
func selfProcStart(t *testing.T) uint64 {
	t.Helper()
	b, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		t.Skipf("no /proc on this platform: %v", err)
	}
	// Field 22 is starttime, counted from the end of the comm field, which may
	// itself contain spaces.
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		t.Fatalf("unexpected /proc/self/stat: %q", s[:min(40, len(s))])
	}
	fields := strings.Fields(s[i+1:])
	// Index 19 counting from just after the comm field — the same index
	// `internal/watch/registry_test.go` reads, and the one `presence.Status`
	// compares against. Getting it wrong does not fail loudly: the process reads
	// *gone*, the watcher reports `ended` before any reconciliation rule is
	// consulted, and the test pins nothing while appearing to run.
	const starttimeOffset = 19
	if len(fields) <= starttimeOffset {
		t.Fatalf("/proc/self/stat has %d fields after comm", len(fields))
	}
	v, err := strconv.ParseUint(fields[starttimeOffset], 10, 64)
	if err != nil {
		t.Fatalf("parsing starttime: %v", err)
	}
	return v
}
