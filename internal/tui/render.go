package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-fleet/internal/api"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	tabActiveStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	cursorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	labelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// renderTabBar renders the top-level tab bar with the active tab highlighted.
func renderTabBar(active tab) string {
	parts := make([]string, len(tabNames))
	for i, name := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		if tab(i) == active {
			parts[i] = tabActiveStyle.Render("[" + label + "]")
		} else {
			parts[i] = dimStyle.Render(" " + label + " ")
		}
	}
	return strings.Join(parts, " ")
}

type column struct {
	header string
	width  int
	// drop is the priority for hiding on narrow terminals: 0 = always kept,
	// higher = dropped first.
	drop   int
	styled bool // color the cell by session status
	cell   func(api.SessionView) string
}

// columns in display order: stable identity on the left, dynamic state on the
// right, with the colored status last.
var columns = []column{
	{"NAME", 22, 0, false, sessionName},
	{"SESSION", 9, 8, false, func(s api.SessionView) string { return shortID(s.ID) }},
	{"DIR", 16, 0, false, func(s api.SessionView) string { return projectName(s.ProjectDir) }},
	{"BRANCH", 16, 7, false, func(s api.SessionView) string { return orDash(s.GitBranch) }},
	{"MACHINE", 10, 4, false, func(s api.SessionView) string { return s.Machine }},
	{"MODEL", 12, 5, false, func(s api.SessionView) string { return shortModel(s.Model) }},
	{"OUT", 8, 3, false, func(s api.SessionView) string { return humanizeTokens(s.Usage.OutputTokens) }},
	{"TOTAL", 9, 2, false, func(s api.SessionView) string { return humanizeTokens(totalTokens(s)) }},
	{"SEEN", 8, 6, false, func(s api.SessionView) string { return clockTime(s.LastSeenAt) }},
	{"STATUS", 13, 0, true, func(s api.SessionView) string { return s.Status }},
}

const colSep = "  "

// renderTable renders the sessions, dropping low-priority columns to fit width
// (width <= 0 means unknown: show everything). The row at index selected is
// marked with a cursor (selected < 0 for none).
func renderTable(sessions []api.SessionView, width, selected int) string {
	cols := visibleColumns(width)

	var b strings.Builder
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = pad(c.header, c.width)
	}
	b.WriteString("  " + headerStyle.Render(strings.Join(headers, colSep)) + "\n")

	for idx, s := range sessions {
		cells := make([]string, len(cols))
		for i, c := range cols {
			cell := pad(c.cell(s), c.width)
			if c.styled {
				cell = statusStyle(s.Status).Render(cell)
			}
			cells[i] = cell
		}
		gutter := "  "
		if idx == selected {
			gutter = cursorStyle.Render("❯ ")
		}
		b.WriteString(gutter + strings.Join(cells, colSep) + "\n")
	}
	return b.String()
}

// renderDetail renders a full-session detail panel.
func renderDetail(s api.SessionView) string {
	name := s.Title
	if name == "" {
		name = s.ID
	}
	lines := []string{
		detailField("Name", name),
		detailField("Session", s.ID),
		detailField("Machine", s.Machine),
		detailField("Directory", s.ProjectDir),
		detailField("Branch", orDash(s.GitBranch)),
		detailField("Model", orDash(s.Model)),
		detailField("Status", s.Status),
		detailField("Last tool", orDash(s.LastTool)),
		detailField("Started", orDash(s.StartedAt)),
		detailField("Last seen", orDash(s.LastSeenAt)),
		detailField("Ended", orDash(s.EndedAt)),
		"",
		detailField("Input", humanizeTokens(s.Usage.InputTokens)),
		detailField("Output", humanizeTokens(s.Usage.OutputTokens)),
		detailField("Cache create", humanizeTokens(s.Usage.CacheCreationTokens)),
		detailField("Cache read", humanizeTokens(s.Usage.CacheReadTokens)),
		detailField("Total", humanizeTokens(totalTokens(s))),
	}
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
	return panel
}

func detailField(label, value string) string {
	return labelStyle.Render(pad(label+":", 14)) + value
}

// visibleColumns returns the columns that fit in width, dropping the
// highest-drop columns first. Columns with drop == 0 are always kept.
func visibleColumns(width int) []column {
	cols := append([]column(nil), columns...)
	if width <= 0 {
		return cols
	}
	for tableWidth(cols) > width {
		idx, maxDrop := -1, 0
		for i, c := range cols {
			if c.drop > maxDrop {
				idx, maxDrop = i, c.drop
			}
		}
		if idx < 0 {
			break // only mandatory columns remain
		}
		cols = append(cols[:idx], cols[idx+1:]...)
	}
	return cols
}

func tableWidth(cols []column) int {
	w := 0
	for i, c := range cols {
		if i > 0 {
			w += len(colSep)
		}
		w += c.width
	}
	return w
}

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "working":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
	case "waiting", "waiting_input":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	case "idle":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // blue
	default:
		return dimStyle // ended / unknown
	}
}

// sessionName is the conversation title, falling back to the short session id.
func sessionName(s api.SessionView) string {
	if s.Title != "" {
		return s.Title
	}
	return shortID(s.ID)
}

func totalTokens(s api.SessionView) int64 {
	return s.Usage.InputTokens + s.Usage.OutputTokens +
		s.Usage.CacheCreationTokens + s.Usage.CacheReadTokens
}

// humanizeTokens renders a token count compactly (e.g. 1234 -> "1.2k").
func humanizeTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// shortModel drops the "claude-" prefix for a compact model label.
func shortModel(m string) string {
	return strings.TrimPrefix(m, "claude-")
}

// clockTime extracts HH:MM:SS from an RFC3339 timestamp.
func clockTime(rfc string) string {
	if i := strings.IndexByte(rfc, 'T'); i >= 0 && len(rfc) >= i+9 {
		return rfc[i+1 : i+9]
	}
	return rfc
}

// shortID returns the first 8 characters of a session id.
func shortID(id string) string {
	r := []rune(id)
	if len(r) > 8 {
		return string(r[:8])
	}
	return id
}

// projectName returns the final path segment of a project directory.
func projectName(dir string) string {
	if dir == "" {
		return "-"
	}
	return filepath.Base(dir)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// pad truncates or right-pads s to exactly w runes.
func pad(s string, w int) string {
	s = truncate(s, w)
	if n := w - len([]rune(s)); n > 0 {
		s += strings.Repeat(" ", n)
	}
	return s
}

func truncate(s string, maxRunes int) string {
	r := []rune(s)
	if maxRunes <= 1 || len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes-1]) + "…"
}
