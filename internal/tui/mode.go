package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
)

// permissionModeLabel maps Claude Code's raw permission mode to a display label
// and color. This is the pinned #303 taxonomy: the canonical field is
// `permissionMode` (the hook payload's `permission_mode` mirrors it); the
// redundant `type:"mode"` line — whose `normal` is the same mode as `default` — is
// ignored upstream. Vigilance rises manual → accept → plan → auto → bypass:
//
//	default            → manual  (grey)   asks for everything — the safe default
//	acceptEdits        → accept  (violet) auto-accepts file edits
//	plan               → plan    (teal)   plans only, waits for your approval
//	auto               → auto    (amber)  runs unattended
//	bypassPermissions  → bypass  (red)    no permission checks at all — watch closely
//
// An unknown non-empty value is shown raw (dimmed), never relabeled "manual" — a
// new mode must not read as the safe default (#304).
func permissionModeLabel(raw string) (label string, style lipgloss.Style) {
	switch raw {
	case "":
		return "-", dimStyle
	case "default":
		return "manual", lipgloss.NewStyle().Foreground(cMuted)
	case "acceptEdits":
		return "accept", lipgloss.NewStyle().Foreground(cAccent2)
	case "plan":
		return "plan", lipgloss.NewStyle().Foreground(cTeal)
	case "auto":
		return "auto", lipgloss.NewStyle().Foreground(cAmber)
	case "bypassPermissions":
		return "bypass", lipgloss.NewStyle().Foreground(cRed)
	default:
		return raw, dimStyle // surface an unrecognized mode, don't fake "manual"
	}
}

// modeCell / modeStyle feed the MODE table column.
func modeCell(s api.SessionView) string { l, _ := permissionModeLabel(s.PermissionMode); return l }
func modeStyle(s api.SessionView) lipgloss.Style {
	_, st := permissionModeLabel(s.PermissionMode)
	return st
}

// modeDetail spells the mode out for the session detail panel.
func modeDetail(s api.SessionView) string {
	switch s.PermissionMode {
	case "":
		return "-"
	case "default":
		return "manual — asks for permission"
	case "acceptEdits":
		return "accept — auto-accepts edits"
	case "plan":
		return "plan — awaiting plan approval"
	case "auto":
		return "auto — runs unattended"
	case "bypassPermissions":
		return "bypass — no permission checks"
	default:
		return s.PermissionMode
	}
}
