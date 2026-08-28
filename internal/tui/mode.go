package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
)

// The permission-mode labels moved to the daemon (ADR-0011, #618): it carries the
// #303 taxonomy and the rule that an unrecognized mode is shown raw rather than
// relabelled "manual", so a mode this build has never heard of cannot read as the
// safe default (#304). What stays here is the color, which is rendering and
// belongs to this client's palette.
//
// Vigilance rises manual → accept → plan → auto → bypass, and the colors say so:
//
//	default            → grey    asks for everything — the safe default
//	acceptEdits        → violet  auto-accepts file edits
//	plan               → teal    plans only, waits for your approval
//	auto               → amber   runs unattended
//	bypassPermissions  → red     no permission checks at all — watch closely
//
// An unrecognized mode is dimmed rather than colored: the cell already says it is
// unknown by showing it raw, and a color would claim it a rung on that scale.
func modeStyle(s api.SessionView) lipgloss.Style {
	switch s.PermissionMode {
	case "default":
		return lipgloss.NewStyle().Foreground(cMuted)
	case "acceptEdits":
		return lipgloss.NewStyle().Foreground(cAccent2)
	case "plan":
		return lipgloss.NewStyle().Foreground(cTeal)
	case "auto":
		return lipgloss.NewStyle().Foreground(cAmber)
	case "bypassPermissions":
		return lipgloss.NewStyle().Foreground(cRed)
	default: // no mode reported, or one this build has never heard of
		return dimStyle
	}
}
