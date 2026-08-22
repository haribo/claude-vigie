package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/version"
)

// Six health indicators used to be scattered across the Sessions tab, each with
// its own glyph, wording and location, and no two designed against each other.
// Five of them asked one question at five granularities — *is what I am looking
// at true?* — and the consequence for the operator was the same every time. The
// sixth was not about vigie at all.
//
// They become one pill in the tab-line corner, and one modal behind `i` holding
// the whole chain between the operator and the truth
// (docs/design/sessions-chrome.md § 3–5, #494).

// stateKey is the key that opens and closes the state modal, on every tab.
const stateKey = "i"

// A layer's level. The ordering criterion is deliberately *not* severity: it is
// whether something on screen is false.
//
// A Claude platform outage is severe and stays amber — vigie is correctly
// reporting sessions that are correctly erroring. It is the cold outside, not a
// dead battery in the thermometer.
type level int

const (
	levelOK      level = iota // ● green  — healthy
	levelUnknown              // ◌ grey   — no channel to observe it; neither good nor bad
	levelDegrade              // ◍ amber  — degraded, but nothing displayed is false
	levelBroken               // ○ red    — the screen may be lying
)

// stateRow is one line of the modal: a layer, its level, and why.
type stateRow struct {
	label  string
	level  level
	detail string
}

// glyph renders a level. Shape carries the meaning as well as color, so a
// monochrome terminal and a colorblind operator read the same thing.
func (l level) glyph() string { return l.glyphAt(false) }

// glyphAt renders a level on the given half of the pulse cycle. `dim` swaps the
// color for the second tone of the same hue; the glyph itself never changes and
// is never blanked — unlike the call marker, which substitutes a space. That is
// one of the three things keeping the two animations apart (ADR-0010, #495).
func (l level) glyphAt(dim bool) string {
	if l == levelUnknown {
		return dimStyle.Render("◌")
	}
	return lipgloss.NewStyle().Foreground(levelColor(l, dim)).Render(l.shape())
}

// shape is the glyph alone. It is what a monochrome terminal reads, so it never
// depends on the pulse.
func (l level) shape() string {
	switch l {
	case levelBroken:
		return "○"
	case levelDegrade:
		return "◍"
	case levelUnknown:
		return "◌"
	default:
		return "●"
	}
}

// levelColor is the tone a level renders in, on either half of the pulse cycle.
// Green never animates — a healthy state is the default and needs no alarm — so
// it answers the same color for both halves.
//
// Split out from the rendering on purpose: a terminal with color disabled (every
// test run, among others) renders both halves to the same bare glyph, so an
// assertion on the rendered string would pass whether or not the pulse exists.
func levelColor(l level, dim bool) lipgloss.AdaptiveColor {
	switch l {
	case levelBroken:
		if dim {
			return cRedDim
		}
		return cRed
	case levelDegrade:
		if dim {
			return cAmberDim
		}
		return cAmber
	default:
		return cGreen
	}
}

// pulseInterval is the half-period of the state pulse: one full cycle every two
// seconds, i.e. 0.5 Hz.
//
// Four times slower than the call marker's blinkInterval, and deliberately so:
// the cadence is what separates "come now" from "still broken". There is a cost
// argument too — the tick is only scheduled while something animates, so a
// long-lived degraded state would otherwise pin the TUI to a permanent 500 ms
// redraw loop, on a tool meant to stay open all day. 0.5 Hz is also well under
// WCAG 2.3.1's three-flashes-per-second ceiling, which is why this cadence may be
// slowed and never sped up (#495).
const pulseInterval = time.Second

// pulsing reports whether the pill is animating — the only condition under which
// the pulse tick is scheduled at all. A degraded pill breathes, amber and red
// both; a healthy one is still.
//
// There is no preference to mute it. With no text beside the pill the pulse *is*
// the alert, so muting it would make a degraded state completely silent — the
// same reasoning that removed prefs.blink (#490).
func (m model) pulsing() bool { return m.stateLevel() != levelOK }

// statePill is the tab line's trailing corner: the `[i]` keycap that opens the
// modal, then the pill. It never changes width — no text ever appears beside it,
// so the table below never jumps.
func (m model) statePill() string {
	return keycapStyle.Render(" "+stateKey+" ") + " " + m.stateLevel().glyphAt(m.pulseOn)
}

// linkDown reports whether the TUI has lost its channel to the vigie server. It
// is the observation everything below it depends on: `sseLive` is a past
// observation, while a failing poll is present-tense proof the server is out of
// reach, so the poll outranks a stale "connected" (#457).
func (m model) linkDown() bool { return m.err != nil }

// stateRows is the whole observation chain, ordered by dependency: what the TUI
// establishes itself, then what only transits through the server, then what it
// holds locally.
//
// The split into red / grey / amber is by *who observes what*. When the server is
// unreachable the TUI loses the ability to observe, not merely the data — and
// showing the last known watcher or platform value as if it were current is
// exactly the lie #449 and #456 exist to prevent.
func (m model) stateRows() []stateRow {
	down := m.linkDown()
	rows := []stateRow{m.serverRow(), m.sessionsRow()}

	// Everything the server relays. No channel, no information.
	if down {
		for _, label := range []string{"watcher", "claude platform", "client / daemon"} {
			rows = append(rows, stateRow{label, levelUnknown, "unknown · needs the server"})
		}
	} else {
		rows = append(rows, m.watcherRow(), m.platformRow(), m.versionRow())
	}

	// Held locally with a timestamp: its age is known even offline, and greying it
	// would hide the one thing still true about it.
	rows = append(rows, m.usageRow())
	return rows
}

func (m model) serverRow() stateRow {
	switch {
	case m.linkDown():
		return stateRow{"vigie server", levelBroken, "offline · " + m.err.Error()}
	case m.sseLive:
		return stateRow{"vigie server", levelOK, "connected · live stream"}
	default:
		// The poll is reaching the server, the stream is not: honestly degraded,
		// and nothing on screen is false — updates simply arrive on the 5 s poll.
		return stateRow{"vigie server", levelDegrade, "reconnecting · polling every 5s"}
	}
}

func (m model) sessionsRow() stateRow {
	if m.refreshFailed[srcSessions] {
		return stateRow{"sessions", levelBroken, "frozen · showing last known"}
	}
	return stateRow{"sessions", levelOK, "refreshing"}
}

func (m model) watcherRow() stateRow {
	switch {
	case !m.gotWatcher:
		return stateRow{"watcher", levelUnknown, "unknown · not reported yet"}
	case m.watcherVerdict() == watcherUnreadable:
		// Also red — the statuses may be frozen exactly as they may be with a dead
		// watcher — but the fault is in what vigie recorded, not on that machine, so
		// the detail says so rather than sending the operator to the wrong host
		// (docs/design/watcher-liveness.md § 5, #600).
		return stateRow{"watcher", levelBroken, "unreadable heartbeat · statuses may be frozen"}
	case m.watcherStale():
		// No watcher means no status is being refreshed: every status on screen may
		// be a frozen one, which is the definition of red here.
		return stateRow{"watcher", levelBroken, "not reporting · statuses may be frozen"}
	default:
		return stateRow{"watcher", levelOK, "reporting"}
	}
}

func (m model) platformRow() stateRow {
	if !platformKnown(m.platform) {
		return stateRow{"claude platform", levelUnknown, "unknown · not reported yet"}
	}
	_, word, _ := platformDisplay(m.platform)
	if m.platform.Indicator == "none" {
		return stateRow{"claude platform", levelOK, word}
	}
	// Severe, and never red: vigie reports a broken platform perfectly well.
	return stateRow{"claude platform", levelDegrade, word + " · sessions may fail"}
}

func (m model) versionRow() stateRow {
	daemon := m.daemonVersion.Version
	if daemon == "" {
		return stateRow{"client / daemon", levelUnknown, "unknown · not reached yet"}
	}
	if version.Match(version.Version, version.Commit, daemon, m.daemonVersion.Commit) {
		return stateRow{"client / daemon", levelOK, version.Version}
	}
	// Buried in Settings until now, though a drift fails confusingly on a fleet
	// (#341). Nothing on screen is false, so it is amber, not red.
	return stateRow{"client / daemon", levelDegrade,
		fmt.Sprintf("drift · client %s, daemon %s", version.Version, daemon)}
}

func (m model) usageRow() stateRow {
	if m.usage.FetchedAt == "" {
		return stateRow{"usage snapshot", levelUnknown, "unknown · not fetched yet"}
	}
	t, err := parseTime(m.usage.FetchedAt)
	if err != nil {
		return stateRow{"usage snapshot", levelUnknown, "unknown · unreadable timestamp"}
	}
	age := m.now().Sub(t)
	if age < 10*time.Minute && !m.refreshFailed[srcUsage] {
		return stateRow{"usage snapshot", levelOK, "fresh"}
	}
	detail := humanAge(age) + " old"
	if m.refreshFailed[srcUsage] || m.linkDown() {
		detail += " · cannot refresh"
	}
	return stateRow{"usage snapshot", levelDegrade, detail}
}

// stateLevel is the pill: the worst row wins, except that an unknown is the
// *absence* of a level and never colors it by itself. What turns the pill red
// when three rows are grey is the observed failure of the link above them.
func (m model) stateLevel() level {
	worst := levelOK
	for _, r := range m.stateRows() {
		if r.level == levelUnknown {
			continue
		}
		if r.level > worst {
			worst = r.level
		}
	}
	return worst
}

// renderState draws the state modal: the whole observation chain, one line per
// layer, in dependency order. A reader can see why three rows are grey by reading
// the two above them.
func renderState(rows []stateRow, width int) string {
	labelW := 0
	for _, r := range rows {
		if w := len([]rune(r.label)); w > labelW {
			labelW = w
		}
	}
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, r.level.glyph()+" "+labelStyle.Render(pad(r.label, labelW))+"   "+dimStyle.Render(r.detail))
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cMuted).
		Padding(1, 3).
		Render(strings.Join(lines, "\n"))
	title := headerStyle.Render("State")
	closeHint := dimStyle.Render(stateKey + " or esc to close")
	return clampWidth(title+"\n"+box+"\n"+closeHint, width)
}

// humanAge renders a duration the way the modal reads it: minutes up to an hour,
// then hours.
func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}
