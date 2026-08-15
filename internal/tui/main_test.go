package tui

import (
	"os"
	"strings"
	"testing"
)

// TestMain isolates the whole package from the operator's home directory.
//
// `TestGroupToggleCycles` builds a zero-value model and sends `g`, which since
// #240 saves the view preferences — into the **real** `~/.config/vigie/tui.toml`,
// because that test never redirected `XDG_CONFIG_HOME`. A zero-value `prefs` is
// not `defaultPrefs()`: it carries no column order and no hidden set, so every
// `just code-check` rewrote the operator's file with `column_order = []` and lost
// their layout (#479).
//
// The fix is here rather than on that one test on purpose. Any test in this
// package can reach the preferences through a keystroke, and the next one to do so
// would leak again; isolating the package makes that impossible instead of
// unlikely.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "vigie-tui-tests")
	if err != nil {
		panic("isolating the test home: " + err.Error())
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// Both are needed: the preferences resolve through XDG_CONFIG_HOME, while
	// presence and the compaction markers derive their paths from HOME alone
	// (ADR-0006).
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

// TestTheTestHomeIsIsolated is the guard for the guard: if TestMain above is ever
// removed or weakened, this fails instead of the operator's preferences being
// silently rewritten the next time someone runs the suite (#479).
func TestTheTestHomeIsIsolated(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if home == "" || !strings.HasPrefix(home, os.TempDir()) {
		t.Fatalf("HOME is %q, which is not a temporary directory — this package writes preferences and presence", home)
	}
	p, err := prefsPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p, os.TempDir()) {
		t.Errorf("preferences would be written to %q, outside the test sandbox", p)
	}
}
