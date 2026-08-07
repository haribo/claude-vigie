package client

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	f()
	_ = w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// TestHooksUsage is the #354 regression: the usage must not advertise the removed
// --detailed flag, and an unknown flag must print the real usage (not the empty
// "Usage of hooks:").
func TestHooksUsage(t *testing.T) {
	usage := captureStderr(t, hooksUsage)
	if strings.Contains(usage, "--detailed") {
		t.Errorf("usage still advertises the removed --detailed flag:\n%s", usage)
	}
	if !strings.Contains(usage, "vigie hooks install") {
		t.Errorf("usage should document `vigie hooks install`:\n%s", usage)
	}

	// An unknown flag: exit 2, and the real usage (has "uninstall") is printed.
	var code int
	out := captureStderr(t, func() { code = runHooks([]string{"install", "--detailed"}) })
	if code != 2 {
		t.Errorf("runHooks(install --detailed) = %d, want 2", code)
	}
	if !strings.Contains(out, "vigie hooks uninstall") {
		t.Errorf("an unknown flag should print the real usage, got:\n%s", out)
	}
}
