package tui

import "github.com/charmbracelet/lipgloss"

// Truecolor "radar" palette, adaptive to light and dark terminals: Lip Gloss
// picks the right side from the detected terminal background.
var (
	cAccent  = lipgloss.AdaptiveColor{Light: "#0284c7", Dark: "#38bdf8"} // sky
	cAccent2 = lipgloss.AdaptiveColor{Light: "#9333ea", Dark: "#c084fc"} // violet
	cMuted   = lipgloss.AdaptiveColor{Light: "#94a3b8", Dark: "#64748b"} // slate
	cText    = lipgloss.AdaptiveColor{Light: "#0f172a", Dark: "#e2e8f0"}
	cSurface = lipgloss.AdaptiveColor{Light: "#e2e8f0", Dark: "#1e293b"}
	cGreen   = lipgloss.AdaptiveColor{Light: "#16a34a", Dark: "#4ade80"}
	cAmber   = lipgloss.AdaptiveColor{Light: "#b45309", Dark: "#fbbf24"}
	cBlue    = lipgloss.AdaptiveColor{Light: "#2563eb", Dark: "#60a5fa"}
	cRed     = lipgloss.AdaptiveColor{Light: "#dc2626", Dark: "#f87171"}
	cOrange  = lipgloss.AdaptiveColor{Light: "#ea580c", Dark: "#fb923c"} // stalled — a hung tool
	cTeal    = lipgloss.AdaptiveColor{Light: "#0d9488", Dark: "#2dd4bf"} // plan mode (#304)
	cSel     = lipgloss.AdaptiveColor{Light: "#e0f2fe", Dark: "#16273c"} // selected-row fill
)

// The second tone of the state pill's pulse (#495). Each stays inside its own
// hue — an amber drifting toward orange, or a red toward pink, would move the
// meaning, since color already carries severity.
//
// The direction differs by theme because both move toward the ground: on a light
// terminal the tone lightens, on a dark one it darkens. That cannot be computed
// — the TUI does not know the terminal's background color — so each pair is
// chosen by hand and checked by eye in a real terminal.
var (
	cAmberDim = lipgloss.AdaptiveColor{Light: "#d08a4a", Dark: "#c08f2a"}
	cRedDim   = lipgloss.AdaptiveColor{Light: "#ec7070", Dark: "#bd5555"}
)
