package tui

import (
	"os"
	"os/exec"
	"sort"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/status"
)

// attentionStatuses are the states that call for the operator — the session is
// blocked and needs a human: it is waiting on input, it errored, or a tool hung
// (stalled, #256).
// The set itself lives in internal/status, so the TUI and the GNOME indicator
// cannot disagree about when the operator should be interrupted (#466).
func isAttentionStatus(s string) bool { return status.NeedsAttention(s) }

func isAttention(s string) bool { return isAttentionStatus(s) }

// nextAttention returns the id of the session that has been waiting on the
// operator longest — the oldest by StatusChangedAt among the attention states —
// so the jump-to-next hotkey (#261) dequeues the attention queue oldest-first.
// LastSeenAt is useless here: the watcher refreshes it every tick, so a session's
// StatusChangedAt is the only measure of how long it has been waiting. Empty when
// nothing needs attention.
func nextAttention(sessions []api.SessionView) string {
	// A raised call jumps ahead of every inferred attention state: the session
	// said so itself, where waiting/error/stalled are deductions (ADR-0010, #389).
	var called []api.SessionView
	for _, s := range sessions {
		if hasCall(s) {
			called = append(called, s)
		}
	}
	if len(called) > 0 {
		sort.Slice(called, func(i, j int) bool { return called[i].CallAt < called[j].CallAt })
		return called[0].ID
	}

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
	prev, prevCall := m.sess.prevStatus, m.sess.prevCall
	m.sess.prevStatus = make(map[string]string, len(next))
	m.sess.prevCall = make(map[string]bool, len(next))
	for _, s := range next {
		_, known := prev[s.ID] // a session first seen at startup never notifies
		calling := hasCall(s)
		if !m.focus.suppressesNotifications() && m.prefs.notify {
			switch {
			case known && calling && !prevCall[s.ID]:
				// A raised call is exactly what this notification is for (#260).
				notifyFn(sessionName(s), "calling you")
			case prev[s.ID] == "working" && isAttention(s.Status):
				notifyFn(sessionName(s), s.Status)
			}
		}
		m.sess.prevStatus[s.ID] = s.Status
		m.sess.prevCall[s.ID] = calling
	}
	return m
}

// displayEnv returns the graphical session this process can reach, or "".
//
//nolint:forbidigo // DISPLAY/WAYLAND_DISPLAY are a platform probe, not config
func displayEnv() string {
	if d := os.Getenv("DISPLAY"); d != "" {
		return d
	}
	return os.Getenv("WAYLAND_DISPLAY")
}

// notifyFn is the notification sink, indirected so tests can capture transitions
// without spawning notify-send.
var notifyFn = notifySend

// notifySend delivers a best-effort desktop notification. It runs only under a
// graphical session (never SSH/headless), never critical, never with sound, and
// never blocks — a missing notify-send or display simply does nothing, so the
// render loop can never fail because of it.
func notifySend(name, status string) {
	// Graphical-session detection, not app config, so the notifier no-ops under
	// SSH/headless. Shared with the availability probe (#411) so what the Settings
	// tab reports and what the notifier actually does cannot drift apart.
	if displayEnv() == "" {
		return
	}
	// Fixed argv, no shell: name/status are notify-send's title/body text, not a
	// command, so there is no injection surface. Start (not Run): fire-and-forget.
	//nolint:gosec // constant command, arguments are display text only
	_ = exec.Command("notify-send", "-u", "normal", "-a", "vigie", "vigie — "+name, "is "+status).Start()
}
