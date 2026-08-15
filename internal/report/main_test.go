package report

import (
	"os"
	"strings"
	"testing"
)

// TestMain isolates the whole package from the operator's home directory.
//
// `TestRunPostsReport` sends a report without redirecting HOME, and the reporter
// records the session→process mapping as it goes (ADR-0006). It therefore wrote
// `s1.json` into the operator's real `~/.local/state/vigie/sessions/`, next to the
// files of live sessions — a phantom session in the state the watcher reads (#479).
//
// The fix is here rather than on that one test: every code path in this package
// touches presence or the compaction markers, so the next test to forget the
// redirect would leak the same way.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "vigie-report-tests")
	if err != nil {
		panic("isolating the test home: " + err.Error())
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// Presence and the compaction markers derive their paths from HOME alone
	// (ADR-0006); the client config resolves through XDG_CONFIG_HOME.
	for _, kv := range [][2]string{
		{"HOME", dir},
		{"XDG_CONFIG_HOME", dir + "/config"},
		{"XDG_STATE_HOME", dir + "/state"},
	} {
		if err := os.Setenv(kv[0], kv[1]); err != nil {
			panic("isolating the test home: " + err.Error())
		}
	}

	os.Exit(m.Run())
}

// TestTheTestHomeIsIsolated is the guard for the guard: without it, removing
// TestMain above would put `s1.json` back into the operator's live presence
// directory, silently (#479).
func TestTheTestHomeIsIsolated(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if home == "" || !strings.HasPrefix(home, os.TempDir()) {
		t.Fatalf("HOME is %q, which is not a temporary directory — this package records presence", home)
	}
}
