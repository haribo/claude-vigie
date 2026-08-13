package tui

import "os/exec"

// Desktop notifications had three independent ways of never firing, all silent
// (#411). Two are addressed here — an operator can now read *why* nothing
// arrives — and the third, the focus assumption, is fixed in the model.

// focusState is what we know about the terminal's focus. It is deliberately
// three-valued: `focused` used to start at true and be corrected by the
// terminal's focus events, so a terminal or multiplexer that never reports them
// left it true **forever** and suppressed every notification, permanently and
// invisibly. Not knowing must not be mistaken for "the operator is watching".
type focusState int

const (
	focusUnknown focusState = iota // no focus event has ever arrived
	focusOn
	focusOff
)

// suppressesNotifications reports whether the operator is known to be looking at
// the dashboard, which is the only case where a notification is redundant.
// Unknown deliberately does *not* suppress: a notification while you are already
// watching is a small annoyance, never receiving one is a broken feature.
func (f focusState) suppressesNotifications() bool { return f == focusOn }

// notifyStatus is why notifications will or will not reach the operator.
type notifyStatus struct {
	enabled bool   // the preference
	usable  bool   // the machine can actually deliver one
	reason  string // why not, when usable is false
}

// label renders the status for the Settings tab, so the answer to "why am I not
// being notified" is on screen rather than in the source.
func (n notifyStatus) label() string {
	switch {
	case !n.enabled:
		return "off"
	case !n.usable:
		return "on — " + n.reason
	default:
		return "on"
	}
}

// lookPath is a seam: tests decide whether notify-send exists.
var lookPath = exec.LookPath

// hasDisplay is a seam over the graphical-session probe.
var hasDisplay = func() bool { return displayEnv() != "" }

// notifyAvailability probes, once, whether a notification could be delivered at
// all: notifySend needs a graphical session and the notify-send binary, and it
// discards the outcome of both — so without this probe a machine missing either
// behaves exactly like one where nothing happened.
func notifyAvailability(enabled bool) notifyStatus {
	if !hasDisplay() {
		return notifyStatus{enabled: enabled, reason: "no graphical session"}
	}
	if _, err := lookPath("notify-send"); err != nil {
		return notifyStatus{enabled: enabled, reason: "notify-send not installed"}
	}
	return notifyStatus{enabled: enabled, usable: true}
}
