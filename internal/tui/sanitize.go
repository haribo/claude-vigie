package tui

import (
	"strings"
	"unicode"

	"github.com/haribo/claude-vigie/internal/api"
)

// A terminal does not merely display what it is sent: some sequences are commands
// to it — clear the screen, move the cursor, set the window title, write the
// clipboard (OSC 52). The strings vigie relays are transcript-derived, so a
// session's own title or activity could act on the operator's terminal at every
// refresh. The web dashboard has escaped its side since #161 for exactly this
// reason; the terminal had nothing (#529).
//
// This is the one place vigie's observe-only stance stops protecting the
// operator: it reads faithfully and hands the result to a program that executes
// it (ADR-0005).

// replacement stands in for a character that was removed. A visible mark rather
// than a deletion: a title made only of control characters would otherwise render
// as an empty line, with nothing saying anything had been there. It is one cell
// wide, so no column shifts.
const replacement = '?'

// sanitizeText replaces every control character with `replacement`, leaving all
// printable text — accents, dashes, CJK — untouched.
//
// C0 (including ESC), DEL and C1 all go: C1 alone can introduce a control
// sequence on a terminal that decodes 8-bit controls, so stripping ESC and
// stopping there would leave a narrower version of the same hole.
func sanitizeText(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s // the overwhelming case: no allocation
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return replacement
		}
		return r
	}, s)
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || unicode.Is(unicode.Cf, r)
}

// sanitizeSessions cleans every field a client renders, on the way *in*.
//
// On the way in rather than at render, because lipgloss emits real escape
// sequences of its own for color: stripping at the render layer would erase the
// TUI's own styling. That mistake could not have been caught by a test — color
// is disabled under test, so the render-layer version would have been green while
// production lost every color.
//
// Cleaning once here also means no future render path has to remember: the model
// never holds a string that can act on a terminal.
func sanitizeSessions(sessions []api.SessionView) []api.SessionView {
	out := make([]api.SessionView, len(sessions))
	for i, s := range sessions {
		// The id is not exempt for being an identifier: the server takes it from the
		// report body and checks only that it is non-empty, and the TUI renders it —
		// in full in the detail panel, and as the row's name whenever a session has
		// no title yet. It was the one field left out of the twelve below, which is
		// what made the invariant above false (#540).
		s.ID = sanitizeText(s.ID)
		s.Title = sanitizeText(s.Title)
		s.User = sanitizeText(s.User)
		s.Machine = sanitizeText(s.Machine)
		s.ProjectDir = sanitizeText(s.ProjectDir)
		s.GitBranch = sanitizeText(s.GitBranch)
		s.Model = sanitizeText(s.Model)
		s.Effort = sanitizeText(s.Effort)
		s.PermissionMode = sanitizeText(s.PermissionMode)
		s.LastTool = sanitizeText(s.LastTool)
		s.Detail = sanitizeText(s.Detail)
		s.RemoteURL = sanitizeText(s.RemoteURL)
		s.CallMessage = sanitizeText(s.CallMessage)
		// The derived fields are not clean for being derived: the daemon builds them
		// out of the same transcript-supplied text (ADR-0011, #618), and they are what
		// the table now renders. Sanitizing only the raw halves would leave exactly
		// the hole #540 closed, one field further along.
		s.Name = sanitizeText(s.Name)
		s.Project = sanitizeText(s.Project)
		s.ModelShort = sanitizeText(s.ModelShort)
		s.ModeLabel = sanitizeText(s.ModeLabel)
		s.ModeDetail = sanitizeText(s.ModeDetail)
		s.DetailText = sanitizeText(s.DetailText)
		out[i] = s
	}
	return out
}
