package tui

// A panel keeps its figures when a refresh fails — blanking the screen on a
// transient blip would be worse. But presenting them as current is a lie the
// operator cannot see through: a panel that has been failing to refresh for an
// hour looked exactly like one that is up to date, and an endpoint that never
// answered looked like "no data" (#449).
//
// So the figures stay and the panel says it could not refresh them.

// Data sources that can fail independently of one another.
//
// Sessions were once excluded, on the grounds that their failure already reached
// the operator through `m.err`. That was the wrong reading: `m.err` did not merely
// report the failure, it stood in for the table — one failed poll blanked the
// fleet, which is what a laptop resuming from suspend does every time. The panel
// that matters most was the last one still doing what this file exists to prevent
// (#456).
const (
	srcUsage    = "usage"
	srcStats    = "stats"
	srcSettings = "settings"
	srcWatcher  = "watcher"
	srcPlatform = "platform status"
	srcVersion  = "daemon version"
	srcSessions = "sessions"
)

// markRefresh records the outcome of one source's refresh. A success clears the
// mark, so a recovered endpoint stops being flagged without anything else having
// to notice.
func (m *model) markRefresh(source string, err error) {
	if m.refreshFailed == nil {
		m.refreshFailed = map[string]bool{}
	}
	if err != nil {
		m.refreshFailed[source] = true
		return
	}
	delete(m.refreshFailed, source)
}

// staleNote is the line a panel puts above figures it could not refresh. Empty
// when the last refresh succeeded, so a healthy panel is unchanged.
func (m model) staleNote(sources ...string) string {
	for _, s := range sources {
		if m.refreshFailed[s] {
			return warnStyle.Render("⚠ could not refresh — showing the last known figures") + "\n\n"
		}
	}
	return ""
}

// staleMark is the compact form, for the bottom strip where a whole line would
// not fit.
func (m model) staleMark(sources ...string) string {
	for _, s := range sources {
		if m.refreshFailed[s] {
			return warnStyle.Render(" ⚠")
		}
	}
	return ""
}

// staleReason is the sessions panel's own note. It names the failure rather than
// giving the generic line, because the reason distinguishes a network blip from a
// rejected token — and the table stays on screen underneath either way (#456).
func (m model) staleReason() string {
	if !m.refreshFailed[srcSessions] || m.err == nil {
		return ""
	}
	return warnStyle.Render("⚠ could not refresh — showing the last known sessions: "+m.err.Error()) + "\n"
}
