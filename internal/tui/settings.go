package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/haribo/claude-vigie/internal/version"
)

// settingsCount is the number of editable rows in the Settings tab.
const settingsCount = 4

// retentionRow is the index of the server session-retention row in Settings;
// notifyRow toggles desktop notifications (#260).
const (
	retentionRow = 2
	notifyRow    = 3
)

// settingsView is the Settings tab's own state: the row cursor spanning the
// editable prefs and the column picker below them (#379).
type settingsView struct {
	cursor int
}

// totalSettingsRows is the base preference rows plus one row per column in the
// column picker (#308).
func totalSettingsRows() int { return settingsCount + len(columns) }

func (m model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	onColumn := m.set.cursor >= settingsCount // a column-picker row
	switch msg.String() {
	case "down", "j":
		if m.set.cursor < totalSettingsRows()-1 {
			m.set.cursor++
		}
	case "up", "k":
		if m.set.cursor > 0 {
			m.set.cursor--
		}
	case " ", "enter":
		if onColumn {
			return m.toggleColumnRow(), nil
		}
		return m.editSetting(1)
	case "right":
		if !onColumn {
			return m.editSetting(1)
		}
	case "left":
		if !onColumn {
			return m.editSetting(-1)
		}
	case "[", "shift+up": // reorder the column up
		if onColumn {
			return m.moveColumnRow(-1), nil
		}
	case "]", "shift+down": // reorder the column down
		if onColumn {
			return m.moveColumnRow(1), nil
		}
	}
	return m, nil
}

// columnAtCursor returns the picker column under the settings cursor.
func (m model) columnAtCursor() column {
	return pickerColumns(m.prefs.columnOrder)[m.set.cursor-settingsCount]
}

// toggleColumnRow shows/hides the column under the cursor (mandatory ones can't
// be hidden) and persists (#308).
func (m model) toggleColumnRow() model {
	m.prefs.columnHidden = toggleColumn(m.prefs.columnHidden, m.columnAtCursor().key())
	savePrefs(m.prefs)
	return m
}

// moveColumnRow reorders the column under the cursor, keeping the cursor on it.
func (m model) moveColumnRow(dir int) model {
	key := m.columnAtCursor().key()
	m.prefs.columnOrder = moveColumn(m.prefs.columnOrder, key, dir)
	for i, c := range pickerColumns(m.prefs.columnOrder) {
		if c.key() == key {
			m.set.cursor = settingsCount + i
			break
		}
	}
	savePrefs(m.prefs)
	return m
}

// editSetting changes the selected preference. Local prefs (rows 0-1) are saved
// to tui.toml; the server retention (row 2) is written to the server via a Cmd.
func (m model) editSetting(dir int) (tea.Model, tea.Cmd) {
	switch m.set.cursor {
	case 0:
		m.prefs.hideEnded = !m.prefs.hideEnded
		savePrefs(m.prefs)
	case 1:
		m.prefs.idleHideAfter = cyclePreset(m.prefs.idleHideAfter, dir)
		savePrefs(m.prefs)
	case retentionRow:
		m.serverRetention = cycleRetention(m.serverRetention, dir)
		return m, m.setRetentionCmd(m.serverRetention)
	case notifyRow:
		m.prefs.notify = !m.prefs.notify
		savePrefs(m.prefs)
	}
	return m, nil
}

// renderBuild shows the client and daemon build versions side by side, flagging
// a drift — a local `vigie` that has fallen behind the `vigied` it talks to,
// which fails in confusing ways on a fleet (#341). The version number is the
// primary display; commit and build time are a dim secondary detail.
func (m model) renderBuild() string {
	var b strings.Builder
	b.WriteString(dimStyle.Render("Build") + "\n\n")

	// The version number is the primary line; commit and build time — long, and
	// secondary — go on their own dim line. Each is clamped so the section never
	// overflows a narrow terminal (#341, #331).
	row := func(label, ver, commit, built string) {
		b.WriteString(clampWidth("  "+labelStyle.Render(pad(label, 24))+ver, m.width) + "\n")
		if commit != "" && commit != "none" {
			detail := "commit " + commit
			if built != "" && built != "unknown" {
				detail += " · built " + built
			}
			b.WriteString(clampWidth("    "+dimStyle.Render(detail), m.width) + "\n")
		}
	}
	row("vigie (this client)", version.Version, version.Commit, version.BuildTime)

	if daemon := m.daemonVersion.Version; daemon == "" {
		b.WriteString(clampWidth("  "+labelStyle.Render(pad("vigied (server)", 24))+
			dimStyle.Render("unknown — not reached yet"), m.width) + "\n")
	} else {
		row("vigied (server)", daemon, m.daemonVersion.Commit, m.daemonVersion.BuildTime)
		if daemon != version.Version {
			b.WriteString(warnStyle.Render("  ⚠ client and daemon versions differ") + "\n")
		}
	}
	return b.String()
}

// renderSettings renders the editable preferences: local display prefs (saved
// to tui.toml) and the server-wide retention.
func (m model) renderSettings() string {
	var b strings.Builder
	b.WriteString(m.staleNote(srcSettings, srcVersion))
	// A preferences file that could not be read is kept, not overwritten — and the
	// operator has to be told, or the TUI silently runs on defaults while their
	// settings sit on disk unused (#480).
	if m.prefs.loadFailed != "" {
		b.WriteString(warnStyle.Render("⚠ "+m.prefs.loadFailed+
			" — running on defaults, and leaving the file untouched") + "\n\n")
	}

	// Connection is read-only here: it is set per machine by `vigie init`
	// and shared with the watcher/reporter. Editing it from the TUI would only
	// change this process and leave the watcher on the old server. The token is
	// never shown.
	b.WriteString(dimStyle.Render("Connection") + "\n\n")
	b.WriteString("  " + labelStyle.Render(pad("Server", 24)) + m.serverURL +
		dimStyle.Render("   (read-only — set via `vigie init`)") + "\n\n")

	b.WriteString(m.renderBuild() + "\n")

	b.WriteString(dimStyle.Render("Preferences") + "\n\n")
	rows := []struct {
		label  string
		value  string
		server bool
	}{
		{"Hide ended sessions", onOffLabel(m.prefs.hideEnded), false},
		{"Hide idle after", idleLabel(m.prefs.idleHideAfter), false},
		{"Session retention", retentionLabel(m.serverRetention), true},
		{"Desktop notifications", notifyAvailability(m.prefs.notify).label(), false},
	}
	for i, r := range rows {
		gutter := "  "
		if i == m.set.cursor {
			gutter = cursorStyle.Render("❯ ")
		}
		line := gutter + labelStyle.Render(pad(r.label, 24)) + r.value
		if r.server {
			line += dimStyle.Render("   (server)")
		}
		b.WriteString(line + "\n")
	}

	// Column picker: every column, visible ones first, toggled with space and
	// reordered with [ ] (#308).
	// Width budget: the terminal is width-bound (no horizontal scroll), so show how
	// much the selected columns cost against the available width, and flag the ones
	// the auto-drop cuts off — the drop is never silent (#317).
	active := activeColumns(m.prefs.columnOrder, m.prefs.columnHidden)
	used, avail := rowWidth(active), m.width // rowWidth includes the 2-col gutter
	over := map[string]bool{}
	for _, c := range overflowColumns(active, avail) {
		over[c.key()] = true
	}
	budget := dimStyle.Render(fmt.Sprintf("   width %d/%d", used, avail))
	if used > avail {
		budget = warnStyle.Render(fmt.Sprintf("   width %d/%d — over by %d", used, avail, used-avail))
	}
	header := dimStyle.Render("Columns") +
		dimStyle.Render("   (space: show/hide    [ ] or shift+↑↓: reorder)") + budget
	if m.width > 0 {
		header = lipgloss.NewStyle().Width(m.width).Render(header) // wrap, don't overflow (#329)
	}
	b.WriteString("\n" + header + "\n\n")
	for i, c := range pickerColumns(m.prefs.columnOrder) {
		gutter := "  "
		if settingsCount+i == m.set.cursor {
			gutter = cursorStyle.Render("❯ ")
		}
		box := dimStyle.Render("[ ]")
		if !columnHidden(m.prefs.columnHidden, c.key()) {
			box = statusStyle("working").Render("[x]")
		}
		label := c.header
		suffix := ""
		if mandatoryColumns[c.key()] {
			suffix = " (required)"
		}
		gap := 18 - len(label) - len(suffix)
		if gap < 1 {
			gap = 1
		}
		cost := dimStyle.Render(fmt.Sprintf("w%d", c.width))
		if over[c.key()] {
			cost = warnStyle.Render(fmt.Sprintf("w%d ⚠ cut off", c.width))
		}
		b.WriteString(gutter + box + " " + label + dimStyle.Render(suffix) + strings.Repeat(" ", gap) + cost + "\n")
	}
	return b.String()
}

func retentionLabel(d time.Duration) string {
	if d == 0 {
		return dimStyle.Render("off (keep all)")
	}
	return humanizeDuration(d)
}

func onOffLabel(on bool) string {
	if on {
		return statusStyle("working").Render("on")
	}
	return dimStyle.Render("off")
}

func idleLabel(d time.Duration) string {
	if d == 0 {
		return dimStyle.Render("off (never)")
	}
	return humanizeDuration(d)
}
