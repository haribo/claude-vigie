package tui

import (
	"os"
	"os/exec"
	"sort"

	"github.com/haribo/claude-vigie/internal/api"
)

// attentionStatuses are the states that call for the operator — the session is
// blocked and needs a human: it is waiting on input, it errored, or a tool hung
// (stalled, #256).
var attentionStatuses = map[string]bool{"waiting": true, "error": true, "stalled": true}

func isAttention(status string) bool { return attentionStatuses[status] }

// nextAttention returns the id of the session that has been waiting on the
// operator longest — the oldest by StatusChangedAt among the attention states —
// so the jump-to-next hotkey (#261) dequeues the attention queue oldest-first.
// LastSeenAt is useless here: the watcher refreshes it every tick, so a session's
// StatusChangedAt is the only measure of how long it has been waiting. Empty when
// nothing needs attention.
func nextAttention(sessions []api.SessionView) string {
	var q []api.SessionView
	for _, s := range sessions {
		if isAttention(s.Status) {
			q = append(q, s)
		}
	}
	if len(q) == 0 {
		return ""
	}
	sort.Slice(q, func(i, j int) bool { return q[i].StatusChangedAt < q[j].StatusChangedAt })
	return q[0].ID
}

// withNotifiedTransitions folds the new session list into the model, firing a
// desktop notification for each session that just left `working` for an attention
// state (#260). Edge-triggered off the remembered previous status — one
// notification per transition, re-armed on the next `working`. A session with no
// observed `working` predecessor (empty prevStatus at startup) never notifies, so
// launching the TUI is silent. Suppressed while the TUI has focus (the operator is
// already looking) or notifications are opted out.
func (m model) withNotifiedTransitions(next []api.SessionView) model {
	prev := m.prevStatus
	m.prevStatus = make(map[string]string, len(next))
	for _, s := range next {
		if prev[s.ID] == "working" && isAttention(s.Status) && !m.focused && m.prefs.notify {
			notifyFn(sessionName(s), s.Status)
		}
		m.prevStatus[s.ID] = s.Status
	}
	return m
}

// notifyFn is the notification sink, indirected so tests can capture transitions
// without spawning notify-send.
var notifyFn = notifySend

// unread reports whether an attention session has an unacknowledged change: it is
// in an attention state and its status changed since the operator last opened it.
// Keyed on StatusChangedAt, not LastSeenAt (which the watcher bumps every tick),
// so a session re-marks unread only when it transitions again (#259).
func (m model) unread(s api.SessionView) bool {
	return isAttention(s.Status) && m.seen[s.ID] < s.StatusChangedAt
}

// unreadSet is the id→unread map the table renderer consumes.
func (m model) unreadSet() map[string]bool {
	u := make(map[string]bool, len(m.sessions))
	for _, s := range m.sessions {
		if m.unread(s) {
			u[s.ID] = true
		}
	}
	return u
}

// unreadCount is the number of attention sessions not yet acknowledged.
func (m model) unreadCount() int {
	n := 0
	for _, s := range m.sessions {
		if m.unread(s) {
			n++
		}
	}
	return n
}

// ack marks the session read: opening its detail is the acknowledgement. Mutating
// the shared seen map works through the value receiver (maps are references).
func (m model) ack(id string) model {
	if m.seen == nil {
		m.seen = map[string]string{}
	}
	for _, s := range m.sessions {
		if s.ID == id {
			m.seen[id] = s.StatusChangedAt
		}
	}
	return m
}

// notifySend delivers a best-effort desktop notification. It runs only under a
// graphical session (never SSH/headless), never critical, never with sound, and
// never blocks — a missing notify-send or display simply does nothing, so the
// render loop can never fail because of it.
func notifySend(name, status string) {
	// Graphical-session detection, not app config, so the notifier no-ops under
	// SSH/headless; reading these directly is intentional here.
	//nolint:forbidigo // DISPLAY/WAYLAND_DISPLAY are a platform probe, not config
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return
	}
	// Fixed argv, no shell: name/status are notify-send's title/body text, not a
	// command, so there is no injection surface. Start (not Run): fire-and-forget.
	//nolint:gosec // constant command, arguments are display text only
	_ = exec.Command("notify-send", "-u", "normal", "-a", "vigie", "vigie — "+name, "is "+status).Start()
}
