package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #490. On 0.5.0 a session that raised a call did not blink, and nothing said
// why: the dot was simply lit steadily, indistinguishable from any other row.
// The call pipeline was fine — the operator's ~/.config/vigie/tui.toml carried
// `blink = false`, almost certainly written by the test leak fixed in #479,
// which stamped a zero-value prefs over the real file on every `just code-check`.
//
// The other casualties of that leak were repairable from the Settings tab.
// `blink` was not: it had no control there at all, so its "off" state was
// invisible and unreachable from inside vigie. An accelerator the operator
// cannot see is worse than no accelerator, so the preference is gone rather than
// exposed — a call always blinks.

// The regression, written against the file that caused it.
func TestAStoredBlinkFalseNoLongerSilencesACall(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := prefsPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	// Verbatim from the operator's file.
	if err := os.WriteFile(path, []byte("blink = false\ncall_marker = \"●\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := stubModel()
	m.prefs = loadPrefs()
	m.sessions = []api.SessionView{{ID: "a", Status: "working", CallAt: "2026-08-15T12:00:00Z"}}

	if !m.frame().blinking(m.sessions) {
		t.Fatal("a stored `blink = false` still silences the call marker")
	}
	on := frame{marker: m.prefs.callMarker}
	off := frame{hidden: true, marker: m.prefs.callMarker}
	if on.callDot() == off.callDot() {
		t.Errorf("the marker does not alternate: %q on both half-cycles", on.callDot())
	}
}

// The preference is gone, not defaulted: a file that still carries the key is
// simply ignored, and the next save drops it.
func TestTheBlinkPreferenceIsNoLongerWritten(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := loadPrefs()
	savePrefs(p)

	path, err := prefsPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path) //nolint:gosec // our own temp dir
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); strings.Contains(got, "blink") {
		t.Errorf("the saved preferences still carry a blink key:\n%s", got)
	}
}

// The tick must still exist only while a call is on screen — removing the
// preference must not turn the animation into a permanent redraw loop.
func TestNoTickWithoutACall(t *testing.T) {
	m := stubModel()
	m.sessions = []api.SessionView{{ID: "a", Status: "working"}}
	if m.frame().blinking(m.sessions) {
		t.Error("a table with no call must schedule no tick")
	}
}
