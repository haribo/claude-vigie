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
func (l level) glyph() string {
	switch l {
	case levelBroken:
		return lipgloss.NewStyle().Foreground(cRed).Render("○")
	case levelDegrade:
		return lipgloss.NewStyle().Foreground(cAmber).Render("◍")
	case levelUnknown:
		return dimStyle.Render("◌")
	default:
		return lipgloss.NewStyle().Foreground(cGreen).Render("●")
	}
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

// statePill is the tab line's trailing corner: the `[i]` keycap that opens the
// modal, then the pill. It never changes width — no text ever appears beside it,
// so the table below never jumps.
func (m model) statePill() string {
	return keycapStyle.Render(" "+stateKey+" ") + " " + m.stateLevel().glyph()
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
