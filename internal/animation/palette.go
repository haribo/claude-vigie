package animation

// Palette is the ten colors the drawing uses, named by role. The light and dark
// files are the same template rendered with two of these, which is what makes
// the themes identical by construction rather than by review.
type Palette struct {
	Name       string // "light" | "dark", also the suffix of the clip-path ids
	Accent     string
	Background string
	Panel      string
	Border     string
	Chrome     string
	Dim        string
	Text       string
	Green      string
	Amber      string
	Blue       string
}

var (
	lightPalette = Palette{Name: "light",
		Accent:     "#0284c7",
		Background: "#ffffff",
		Panel:      "#f8fafc",
		Border:     "#e2e8f0",
		Chrome:     "#cbd5e1",
		Dim:        "#94a3b8",
		Text:       "#0f172a",
		Green:      "#16a34a",
		Amber:      "#b45309",
		Blue:       "#2563eb",
	}
	darkPalette = Palette{Name: "dark",
		Accent:     "#38bdf8",
		Background: "#0f172a",
		Panel:      "#111c2e",
		Border:     "#1e293b",
		Chrome:     "#334155",
		Dim:        "#64748b",
		Text:       "#e2e8f0",
		Green:      "#4ade80",
		Amber:      "#fbbf24",
		Blue:       "#60a5fa",
	}
)
