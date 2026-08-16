package tui

import (
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// #529. A terminal does not merely display text: some character sequences are
// commands to it — clear the screen, move the cursor, set the window title, write
// the clipboard (OSC 52). The TUI printed session titles, detail text and call
// messages exactly as they arrived, and those come from the sessions being
// watched. So vigie observed faithfully and then relayed the contents to a
// program that executes them.
//
// The web dashboard has always escaped its side (`esc()`, #161) for the same
// reason. The terminal had nothing.
//
// The cleaning happens on the way IN, not on the way out: lipgloss emits real
// escape sequences of its own for color, and stripping at render would erase the
// TUI's own styling. That could not have been caught by a test — color is
// disabled under test, so a render-layer version would have been green while
// production lost every color.

const evil = "web-app\x1b[2Jgone"

func TestControlSequencesNeverReachTheScreen(t *testing.T) {
	sessions := []api.SessionView{{
		ID: "a", Machine: "m", Status: "working",
		Title:       evil,
		Detail:      "Edit\x1b]52;c;cGF5bG9hZA==\x07x.go",
		GitBranch:   "main\rrewritten",
		ProjectDir:  "/p/\x1b[31mred",
		CallMessage: "done\x1b[2J",
		CallAt:      "2026-08-16T12:00:00Z",
	}}

	m := stubModel()
	m.width, m.height = 200, 40
	m = m.applySessions(sessionsMsg{sessions: sessions, gen: 1})

	for _, view := range []string{m.View(), renderDetail(m.sessions[0])} {
		if strings.ContainsAny(view, "\x1b\r") {
			t.Errorf("a control character reached the screen:\n%q", view)
		}
	}
}

// The text itself survives — this must clean, not censor.
func TestTheReadableTextSurvives(t *testing.T) {
	m := stubModel()
	m = m.applySessions(sessionsMsg{gen: 1, sessions: []api.SessionView{
		{ID: "a", Machine: "m", Status: "working", Title: evil},
	}})
	got := m.sessions[0].Title
	if !strings.HasPrefix(got, "web-app") || !strings.HasSuffix(got, "gone") {
		t.Errorf("title = %q — the readable text was lost with the escape", got)
	}
}

// A removed character leaves a visible mark: a title made only of control
// characters would otherwise render as an empty line, with nothing saying
// something had been there.
func TestARemovedCharacterLeavesAMark(t *testing.T) {
	if got := sanitizeText("\x1b\x1b"); got != "??" {
		t.Errorf("sanitizeText(escapes only) = %q, want ?? — an empty line says nothing was there", got)
	}
	if got := sanitizeText("héllo — ok"); got != "héllo — ok" {
		t.Errorf("ordinary text was altered: %q", got)
	}
}

// The desktop notification reads the sessions before they are stored, so it must
// be fed the cleaned ones too — a notification is another program's input.
func TestTheNotificationPathSeesCleanText(t *testing.T) {
	m := stubModel()
	m.sess.prevStatus = map[string]string{"a": "working"}
	m = m.applySessions(sessionsMsg{gen: 1, sessions: []api.SessionView{
		{ID: "a", Machine: "m", Status: "waiting", Title: evil},
	}})
	if strings.Contains(m.sessions[0].Title, "\x1b") {
		t.Error("the stored session still carries an escape")
	}
}

// #540. The twelve fields above were cleaned and `ID` was not, so the invariant
// stated in sanitize.go — "the model never holds a string that can act on a
// terminal" — was false for the one field nobody thought of.
//
// The id is attacker-chosen in the same sense every other field is: it arrives in
// a report body and the server checks only that it is non-empty
// (internal/server/report.go), unlike `event` and `status`, which are matched
// against closed vocabularies (#515). It reaches the screen by three paths — the
// detail panel prints it in full, `sessionName` falls back to it whenever a
// session has no title yet, and the desktop notification is fed the same name.
//
// The fixtures above all use `ID: "a"`, which is exactly why this went unseen.
const evilID = "sess\x1b]52;c;cGF5bG9hZA==\x07\x1b[2Jid"

func TestAControlSequenceInTheSessionIdNeverReachesTheScreen(t *testing.T) {
	m := stubModel()
	m.width, m.height = 200, 40
	// No Title: the id is what the row is named, which is the ordinary case for a
	// session that has not been titled yet.
	m = m.applySessions(sessionsMsg{gen: 1, sessions: []api.SessionView{
		{ID: evilID, Machine: "m", Status: "working"},
	}})

	if got := m.sessions[0].ID; strings.ContainsAny(got, "\x1b\r") {
		t.Errorf("the stored session id still carries a control character: %q", got)
	}
	for _, view := range []string{m.View(), renderDetail(m.sessions[0])} {
		if strings.ContainsAny(view, "\x1b\r") {
			t.Errorf("a control character reached the screen:\n%q", view)
		}
	}
}

// Cleaning must not rename the session: the id is a key, and replacing a
// character is not the same as dropping it. Length in runes is preserved, so the
// eight-rune row name still cuts where it always did.
func TestTheSessionIdKeepsItsReadableTextAndLength(t *testing.T) {
	got := sanitizeText(evilID)
	if !strings.HasPrefix(got, "sess") || !strings.HasSuffix(got, "id") {
		t.Errorf("id = %q — the readable text was lost with the escape", got)
	}
	if len([]rune(got)) != len([]rune(evilID)) {
		t.Errorf("id length changed: %d runes, want %d", len([]rune(got)), len([]rune(evilID)))
	}
}
