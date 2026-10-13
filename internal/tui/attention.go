package tui

import (
	"os/exec"
	"sort"

	"github.com/haribo/claude-vigie/internal/api"
)

// isAttention reports whether a session's state calls for the operator — it is
// blocked and needs a human: waiting on input, or stopped on an API error.
//
// It listed a third, a session hung on a tool. vigie cannot know a tool is hung —
// the pairing proves a call is outstanding, never that it is stuck — which is
// what [ADR-0012](../../docs/adr/0012-retire-the-stalled-status.md) removed
// `stalled` for.
//
// The daemon decides it (ADR-0011, #617). The set used to live in
// internal/status precisely so the TUI and the GNOME indicator could not
// disagree, and it was still hand-copied into both JavaScript clients; deriving
// it once removes the copies rather than checking them.
//
// A raised call is not part of it — it is not a status but rides alongside one
// (ADR-0010) — so callers that must weigh both read CallAt too.
func isAttention(s api.SessionView) bool { return s.Attention }

// nextAttention returns the id of the session that has been waiting on the
// operator longest — the oldest by StatusChangedAt among the attention states —
// so the jump-to-next hotkey (#261) dequeues the attention queue oldest-first.
// LastSeenAt is useless here: the watcher refreshes it every tick, so a session's
// StatusChangedAt is the only measure of how long it has been waiting. Empty when
// nothing needs attention.
func nextAttention(sessions []api.SessionView) string {
	// A raised call jumps ahead of every inferred attention state: the session
	// said so itself, where waiting and error are deductions (ADR-0010, #389).
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
		if isAttention(s) {
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
// desktop notification for each session that just *entered* the attention set, or
// raised a call (#260). Edge-triggered off the remembered previous state — one
// notification per transition, re-armed when the session leaves the set again.
//
// It used to fire only on `working` → attention, which made the terminal the odd
// one out: the GNOME indicator notifies on any entry, and the README describes
// that. A permission prompt arriving after a turn had finished — the session was
// `idle`, not `working` — was audible on the desktop and silent in the terminal,
// on the one signal the product exists to deliver (#665).
//
// The narrow rule looked like it was buying silence at startup. It was not: that
// comes from `known`, which skips any session seen for the first time, and is
// untouched here. The `working` predecessor bought nothing and cost a real
// notification.
//
// Suppressed while the TUI has focus (the operator is already looking) or
// notifications are opted out.
func (m model) withNotifiedTransitions(next []api.SessionView) model {
	prev, prevCall, prevAttention := m.sess.prevStatus, m.sess.prevCall, m.sess.prevAttention
	m.sess.prevStatus = make(map[string]string, len(next))
	m.sess.prevCall = make(map[string]bool, len(next))
	m.sess.prevAttention = make(map[string]bool, len(next))
	for _, s := range next {
		_, known := prev[s.ID] // a session first seen at startup never notifies
		calling, attention := hasCall(s), isAttention(s)
		if !m.focus.suppressesNotifications() && m.prefs.notify {
			switch {
			case known && calling && !prevCall[s.ID]:
				// A raised call is exactly what this notification is for (#260).
				notifyFn(s.Name, notifyBody(s))
			case known && attention && !prevAttention[s.ID]:
				notifyFn(s.Name, notifyBody(s))
			}
		}
		m.sess.prevStatus[s.ID] = s.Status
		m.sess.prevCall[s.ID] = calling
		m.sess.prevAttention[s.ID] = attention
	}
	return m
}

// notifyBody is what the notification says, from the reason the daemon derived
// (`AttentionReason`).
//
// It used to be the raw status, prefixed with "is" — so a session stopped on a
// 529 was announced as "is error", which is neither English nor an instruction
// (#742). The status is what the session *is*; a notification has to say what is
// being asked.
//
// The wording is the terminal's own. The GNOME toast pairs the same reasons with
// the machine and the branch because it carries no other context; here the board
// is on screen. What the three clients share is the reason, not the sentence.
func notifyBody(s api.SessionView) string {
	switch s.AttentionReason {
	case "call":
		if s.CallMessage != "" {
			return s.CallMessage
		}
		return "calling you"
	case "waiting":
		return "is waiting for input"
	case "error":
		return "hit an API error"
	default:
		return "needs you"
	}
}

// notifyFn is the notification sink, indirected so tests can capture transitions
// without spawning the notifier.
var notifyFn = notifySend

// notifySend delivers a best-effort desktop notification through the platform's
// notifier (`notifyBinary`). It runs only under a graphical session (never
// SSH/headless), never critical, never with sound, and never blocks — a missing
// notifier or display simply does nothing, so the render loop can never fail
// because of it.
func notifySend(name, body string) {
	// Graphical-session detection, not app config, so the notifier no-ops under
	// SSH/headless. Shared with the availability probe (#411) so what the Settings
	// tab reports and what the notifier actually does cannot drift apart.
	if !graphicalSession() {
		return
	}
	startAndReap(notifyCommand("vigie — "+name, body), nil)
}

// startAndReap starts cmd and waits for it somewhere the render loop is not.
//
// `Start` alone leaves the child unreaped — Go's runtime does not do it for you —
// so every attention transition and every raised call added a zombie that lived
// until the TUI exited, on a tool meant to stay open all day. `Run` would fix the
// zombie and introduce a worse bug: it blocks, and this is called from the update
// path. So: start here, wait in a goroutine.
//
// done is nil in production and a signal in tests, which is the only way to
// assert the reap without racing the goroutine that performs it (#560).
func startAndReap(cmd *exec.Cmd, done func()) {
	if err := cmd.Start(); err != nil {
		if done != nil {
			done()
		}
		return
	}
	go func() {
		_ = cmd.Wait()
		if done != nil {
			done()
		}
	}()
}
