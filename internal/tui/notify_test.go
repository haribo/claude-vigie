package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

func withProbes(t *testing.T, display, binary bool) {
	t.Helper()
	origD, origL := hasDisplay, lookPath
	hasDisplay = func() bool { return display }
	lookPath = func(string) (string, error) {
		if binary {
			return "/usr/bin/notify-send", nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { hasDisplay, lookPath = origD, origL })
}

// TestUnknownFocusDoesNotSuppress is the #411 bug. `focused` used to start true
// and be corrected by the terminal's focus events, so a terminal that never
// reports them suppressed every notification forever, silently. Not knowing must
// never be read as "the operator is watching".
func TestUnknownFocusDoesNotSuppress(t *testing.T) {
	if focusUnknown.suppressesNotifications() {
		t.Error("an unobserved focus state suppressed notifications")
	}
	if focusOff.suppressesNotifications() {
		t.Error("a blurred terminal suppressed notifications")
	}
	if !focusOn.suppressesNotifications() {
		t.Error("a focused terminal should suppress: the operator is already looking")
	}
	// The zero value is what a model built without a focus event holds.
	var m model
	if m.focus != focusUnknown {
		t.Errorf("zero focus = %v, want unknown", m.focus)
	}
}

// TestNotifiesWhenFocusWasNeverReported is the same guarantee through the real
// path: a terminal that reports nothing must still notify.
func TestNotifiesWhenFocusWasNeverReported(t *testing.T) {
	var fired []string
	orig := notifyFn
	notifyFn = func(name, status string) { fired = append(fired, name+":"+status) }
	t.Cleanup(func() { notifyFn = orig })

	m := model{prefs: prefs{notify: true}, // focus left at its zero value: unknown
		sess: sessionsView{prevStatus: map[string]string{}, prevCall: map[string]bool{}}}
	m = m.withNotifiedTransitions([]api.SessionView{{ID: "a", Title: "a", Status: "working"}})
	m.withNotifiedTransitions([]api.SessionView{{ID: "a", Title: "a", Status: "waiting"}})

	if len(fired) != 1 {
		t.Errorf("fired %v, want one notification despite never hearing about focus", fired)
	}
}

// TestNotifyAvailabilityNamesTheCause covers the other two silent failures: an
// operator must be able to read why nothing arrives.
func TestNotifyAvailabilityNamesTheCause(t *testing.T) {
	withProbes(t, true, true)
	if got := notifyAvailability(true); !got.usable || got.label() != "on" {
		t.Errorf("all good: label = %q, usable = %v", got.label(), got.usable)
	}
	if got := notifyAvailability(false).label(); got != "off" {
		t.Errorf("disabled: label = %q, want off", got)
	}

	withProbes(t, true, false)
	if got := notifyAvailability(true); got.usable || !strings.Contains(got.label(), "notify-send") {
		t.Errorf("missing binary: label = %q", got.label())
	}

	withProbes(t, false, true)
	if got := notifyAvailability(true); got.usable || !strings.Contains(got.label(), "graphical") {
		t.Errorf("headless: label = %q", got.label())
	}

	// Disabled wins over unusable: the operator's own choice is the first answer.
	withProbes(t, false, false)
	if got := notifyAvailability(false).label(); got != "off" {
		t.Errorf("disabled and unusable: label = %q, want off", got)
	}
}

// TestSettingsShowsWhyNotificationsCannotWork: the reason has to reach the screen,
// not just exist in a struct.
func TestSettingsShowsWhyNotificationsCannotWork(t *testing.T) {
	withProbes(t, true, false)
	m := model{prefs: defaultPrefs(), width: 110}
	out := m.renderSettings()
	if !strings.Contains(out, "notify-send not installed") {
		t.Errorf("the Settings tab does not say why notifications cannot work:\n%s", out)
	}
}
