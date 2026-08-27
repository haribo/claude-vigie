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
		// Validated on ingest against a closed vocabulary, and cleaned anyway: the
		// ranking's own Status is cleaned, and one exemption reasoned two ways in one
		// file is how a reader learns to skip the reasons (#635).
		s.Status = sanitizeText(s.Status)
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
		// The five timestamps. The daemon refuses a report whose timestamp is not an
		// RFC3339 instant (#629), so nothing hostile should arrive — and that is a
		// check in the *other* binary. The invariant stated above is about this
		// model, and a model whose safety depends on a promise made across a network
		// is not the invariant it claims to be. The detail panel is where they
		// surfaced: it printed three of them exactly as the report sent them.
		s.StartedAt = sanitizeText(s.StartedAt)
		s.LastSeenAt = sanitizeText(s.LastSeenAt)
		s.EndedAt = sanitizeText(s.EndedAt)
		s.StatusChangedAt = sanitizeText(s.StatusChangedAt)
		s.CallAt = sanitizeText(s.CallAt)
		out[i] = s
	}
	return out
}

// The other payloads the terminal draws. Sessions were cleaned first and alone,
// which is why #635 existed: the watcher's per-machine builds and the Stats tab's
// figures come from the same reports, through different endpoints, and were
// printed as they arrived.
//
// Each is cleaned at the same seam and for the same reason as sessions — on the
// way in, so no render path has to remember.

// sanitizeWatcherStatus cleans the whole watcher payload. The map *keys* are
// machine names and are printed by the fleet alarm, which names the machines whose
// watcher stopped — so a hostile name reaches the screen through a key even when
// every value is clean.
func sanitizeWatcherStatus(ws api.WatcherStatus) api.WatcherStatus {
	ws.LastSeen = sanitizeText(ws.LastSeen)
	machines := make(map[string]string, len(ws.Machines))
	for k, v := range ws.Machines {
		machines[sanitizeText(k)] = sanitizeText(v)
	}
	versions := make(map[string]api.VersionInfo, len(ws.Versions))
	for k, v := range ws.Versions {
		versions[sanitizeText(k)] = SanitizeVersion(v)
	}
	ws.Machines, ws.Versions = machines, versions
	return ws
}

// SanitizeVersion cleans a build's three strings. It serves the watcher builds
// above, the daemon's own that the Settings tab prints, and the preflight in
// internal/client — which names both in an error printed to the terminal before
// this program even starts, and so needs the same cleaning without the model
// (#635).
func SanitizeVersion(v api.VersionInfo) api.VersionInfo {
	v.Version = sanitizeText(v.Version)
	v.Commit = sanitizeText(v.Commit)
	v.BuildTime = sanitizeText(v.BuildTime)
	return v
}

// sanitizeStats cleans what the Stats tab draws: the model name in the per-model
// legend, and the session, machine and model of every row in the ranking.
func sanitizeStats(st api.StatsResponse) api.StatsResponse {
	daily := make([]api.DailyStat, len(st.Daily))
	for i, d := range st.Daily {
		d.Day = sanitizeText(d.Day)
		d.Model = sanitizeText(d.Model)
		daily[i] = d
	}
	top := make([]api.TopSession, len(st.TopSessions))
	for i, s := range st.TopSessions {
		s.Name = sanitizeText(s.Name)
		s.Machine = sanitizeText(s.Machine)
		s.Model = sanitizeText(s.Model)
		s.Status = sanitizeText(s.Status)
		top[i] = s
	}
	st.Daily, st.TopSessions = daily, top
	return st
}
