package tui

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
)

// #495. The state pill is one character in a corner. Static, it will not be read:
// peripheral vision detects motion and luminance, not hue or shape. And since no
// text ever appears beside it, there is nothing else left to raise the alarm — so
// the pulse *is* the alert (docs/design/sessions-chrome.md § 6).

// A healthy state is the default and needs no alarm.
func TestGreenNeverAnimates(t *testing.T) {
	m := healthyModel()
	if m.pulsing() {
		t.Error("a healthy pill is animating — the alarm would be permanent and mean nothing")
	}
	if _, cmd := m.withPulseTick(); cmd != nil {
		t.Error("a tick was scheduled with every layer healthy")
	}
	if levelColor(levelOK, false) != levelColor(levelOK, true) {
		t.Error("green has two tones — it must not modulate at all")
	}
}

// Amber and red both breathe: "still broken" is worth reading too.
func TestBothDegradedLevelsAnimate(t *testing.T) {
	amber := healthyModel()
	amber.platform = api.PlatformStatus{Indicator: "major"}
	red := healthyModel()
	red.err = errFake{}

	for name, m := range map[string]model{"amber": amber, "red": red} {
		if !m.pulsing() {
			t.Errorf("%s: a degraded pill is not animating", name)
		}
		if _, cmd := m.withPulseTick(); cmd == nil {
			t.Errorf("%s: no tick scheduled for a degraded pill", name)
		}
		lvl := m.stateLevel()
		// On the color itself, not on the rendered string: a terminal with color
		// disabled renders both halves to the same bare glyph, so asserting on the
		// render would pass whether or not the pulse exists.
		if levelColor(lvl, false) == levelColor(lvl, true) {
			t.Errorf("%s: both half-cycles use one color — nothing would move", name)
		}
		if lvl.shape() != m.stateLevel().shape() {
			t.Errorf("%s: the shape changed with the pulse", name)
		}
	}
}

// The glyph is present on both halves. The call marker substitutes a blank on its
// off half-cycle; the pulse only modulates a color, which is one of the three
// things keeping the two animations apart.
func TestThePulseNeverBlanksTheGlyph(t *testing.T) {
	for _, lvl := range []level{levelOK, levelUnknown, levelDegrade, levelBroken} {
		for _, dim := range []bool{false, true} {
			if got := lvl.glyphAt(dim); !hasGlyph(got) {
				t.Errorf("level %v dim=%v rendered %q — the glyph must never disappear", lvl, dim, got)
			}
		}
	}
}

func hasGlyph(s string) bool {
	for _, r := range s {
		switch r {
		case '●', '◍', '○', '◌':
			return true
		}
	}
	return false
}

// The cadence is what separates "come now" from "still broken", so the two must
// not be tied together. It is also the reason the pulse may be slowed and never
// sped up: 0.5 Hz sits well under WCAG 2.3.1's three-flashes-per-second ceiling.
func TestTheCadenceIsIndependentOfTheCallMarker(t *testing.T) {
	if pulseInterval == blinkInterval {
		t.Error("the pulse shares the call marker's cadence — the two alerts would read as one")
	}
	if pulseInterval < 2*blinkInterval {
		t.Errorf("pulseInterval = %s against blinkInterval %s: too close to tell apart", pulseInterval, blinkInterval)
	}
	if cycle := 2 * pulseInterval; cycle < 2*time.Second {
		t.Errorf("the full cycle is %s; it must stay at or below 0.5 Hz (WCAG 2.3.1)", cycle)
	}
}

// The two animations are independent in both directions: a call in the table
// must not decide whether a degraded pill breathes, and a healthy pill must not
// stop a call from blinking.
func TestTheTwoAnimationsDoNotGateEachOther(t *testing.T) {
	// Degraded pill, no call on screen.
	m := healthyModel()
	m.err = errFake{}
	m.sessions = []api.SessionView{{ID: "a", Status: "idle"}}
	if _, cmd := m.withPulseTick(); cmd == nil {
		t.Error("no call on screen stopped the state pulse")
	}

	// A call, and a perfectly healthy pill.
	c := healthyModel()
	c.sessions = []api.SessionView{{ID: "a", Status: "idle", CallAt: "2026-08-16T12:00:00Z"}}
	if _, cmd := c.withBlinkTick(); cmd == nil {
		t.Error("a healthy pill stopped a call from blinking")
	}
}

// The tick must not outlive its reason, or a recovered fleet keeps the TUI in a
// redraw loop for as long as it stays open.
func TestThePulseStopsWhenTheStateRecovers(t *testing.T) {
	m := healthyModel()
	m.err = errFake{}
	started, _ := m.withPulseTick()
	m = started.(model)
	if !m.pulseTicking {
		t.Fatal("the tick was not marked in flight")
	}
	if _, cmd := m.withPulseTick(); cmd != nil {
		t.Error("a second tick was stacked on the first")
	}

	m.err = nil // the link comes back
	next, cmd := m.Update(pulseMsg{})
	got := next.(model)
	if cmd != nil {
		t.Error("the tick rescheduled itself on a healthy pill")
	}
	if got.pulseTicking || got.pulseOn {
		t.Errorf("animation state not reset: ticking=%v on=%v", got.pulseTicking, got.pulseOn)
	}
}

// The corner must not change width between the two half-cycles either, or the
// tab line would jitter twice a second.
func TestThePulseKeepsTheCornerWidth(t *testing.T) {
	m := healthyModel()
	m.err = errFake{}
	on, off := m.statePill(), m
	off.pulseOn = true
	if len([]rune(on)) != len([]rune(off.statePill())) {
		t.Errorf("the corner changes width across the pulse: %q vs %q", on, off.statePill())
	}
}

// The modulation stays inside its own hue. An amber drifting toward orange, or a
// red toward pink, moves the meaning — color already carries severity.
func TestThePulseStaysWithinItsHue(t *testing.T) {
	for _, c := range []struct {
		name      string
		full, dim lipgloss.AdaptiveColor
	}{
		{"amber", cAmber, cAmberDim},
		{"red", cRed, cRedDim},
	} {
		for _, theme := range []struct {
			name      string
			full, dim string
		}{
			{"light", c.full.Light, c.dim.Light},
			{"dark", c.full.Dark, c.dim.Dark},
		} {
			h1, err1 := hueOf(theme.full)
			h2, err2 := hueOf(theme.dim)
			if err1 != nil || err2 != nil {
				t.Fatalf("%s/%s: unparsable color %q or %q", c.name, theme.name, theme.full, theme.dim)
			}
			if d := hueDistance(h1, h2); d > 20 {
				t.Errorf("%s/%s: the second tone is %.0f° away (%s → %s) — that is a different color, not a pulse",
					c.name, theme.name, d, theme.full, theme.dim)
			}
			if theme.full == theme.dim {
				t.Errorf("%s/%s: the two tones are identical", c.name, theme.name)
			}
		}
	}
}

// hueOf returns the HSV hue, in degrees, of a #rrggbb string.
func hueOf(hex string) (float64, error) {
	var r, g, b int
	if _, err := fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b); err != nil {
		return 0, err
	}
	maxV := math.Max(float64(r), math.Max(float64(g), float64(b)))
	minV := math.Min(float64(r), math.Min(float64(g), float64(b)))
	if maxV == minV {
		return 0, nil
	}
	d := maxV - minV
	var h float64
	switch maxV {
	case float64(r):
		h = math.Mod((float64(g)-float64(b))/d, 6)
	case float64(g):
		h = (float64(b)-float64(r))/d + 2
	default:
		h = (float64(r)-float64(g))/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, nil
}

// hueDistance is the shorter way round the color wheel.
func hueDistance(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > 180 {
		d = 360 - d
	}
	return d
}
