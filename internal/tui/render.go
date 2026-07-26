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
)

var columns = []struct {
	name  string
	width int
}{
	{"STATUS", 13},
	{"SESSION", 9},
	{"MACHINE", 10},
	{"PROJECT", 18},
	{"BRANCH", 16},
	{"MODEL", 12},
	{"OUT", 8},
	{"TOTAL", 9},
	{"SEEN", 8},
}

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "working":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
	case "waiting_input":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	case "idle":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // blue
	default:
		return dimStyle // ended / unknown
	}
}

// renderTable renders the sessions as a fixed-width table.
func renderTable(sessions []api.SessionView) string {
	var b strings.Builder

	headerCells := make([]string, len(columns))
	for i, c := range columns {
		headerCells[i] = pad(c.name, c.width)
	}
	b.WriteString(headerStyle.Render(strings.Join(headerCells, "  ")) + "\n")

	for _, s := range sessions {
		total := s.Usage.InputTokens + s.Usage.OutputTokens +
			s.Usage.CacheCreationTokens + s.Usage.CacheReadTokens
		cells := []string{
			statusStyle(s.Status).Render(pad(s.Status, columns[0].width)),
			pad(shortID(s.ID), columns[1].width),
			pad(s.Machine, columns[2].width),
			pad(projectName(s.ProjectDir), columns[3].width),
			pad(orDash(s.GitBranch), columns[4].width),
			pad(shortModel(s.Model), columns[5].width),
			pad(humanizeTokens(s.Usage.OutputTokens), columns[6].width),
			pad(humanizeTokens(total), columns[7].width),
			pad(clockTime(s.LastSeenAt), columns[8].width),
		}
		b.WriteString(strings.Join(cells, "  ") + "\n")
	}
	return b.String()
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
