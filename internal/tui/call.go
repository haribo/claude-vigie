package tui

import (
	"strings"
	"time"
	"unicode"

	"github.com/haribo/claude-vigie/internal/api"
)

// A session-raised call (ADR-0010) is found by motion, not by reading: with many
// rows open the operator does not read them all, and the palette already carries
// the status vocabulary. The status dot is the one glyph in a row that can be
// animated without destroying information — the status is spelled out in full
// right beside it (#389).

// blinkInterval is the half-period of the marker: one full on/off cycle per
// second, i.e. 1 Hz — inside WCAG 2.3.1's three-flashes-per-second ceiling. The
// ambient poll (pollInterval) stays at 5 s; this tick runs only while something
// is actually blinking.
const blinkInterval = 500 * time.Millisecond

// defaultCallMarker is the glyph drawn for a calling session's status dot. It is
// the same `●` as every other row by default (ADR-0010: no new glyph); it is
// configurable only because a font may not carry a given code point.
const defaultCallMarker = "●"

// frame is the per-render animation state. The field says when the marker is
// *hidden* rather than shown, so a zero frame renders a steady, fully-lit table
// — which is what the string wrappers and every static test want.
type frame struct {
	hidden bool   // the calling marker is on its blank half-cycle
	marker string // glyph for a calling session's dot
}

// hasCall reports whether the session is calling the operator. CallAt is what
// marks it: the message is optional (#388).
func hasCall(s api.SessionView) bool { return s.CallAt != "" }

// callCount counts the calling sessions in the list.
func callCount(sessions []api.SessionView) int {
	n := 0
	for _, s := range sessions {
		if hasCall(s) {
			n++
		}
	}
	return n
}

// blinking reports whether anything on screen is animating — the only condition
// under which the blink tick is scheduled at all.
func (f frame) blinking(sessions []api.SessionView) bool {
	return callCount(sessions) > 0
}

// callDot returns the leading glyph for a calling session's status cell: the
// marker on the visible half-cycle, a space on the other. Two hard states, never
// a fade — a terminal draws discrete frames, and a gradient would mean repainting
// the table at animation speed. The blank keeps the cell's width, so no column
// ever shifts.
func (f frame) callDot() string {
	if f.hidden {
		return " "
	}
	if f.marker == "" {
		return defaultCallMarker
	}
	return f.marker
}

// statusDots are the leading glyphs a status cell can start with: the live dot
// and the dotted "no fresh signal" ring (#285).
var statusDots = []string{"●", "◌"}

// replaceLeadingDot swaps the cell's leading status glyph for dot, leaving the
// rest — the status word, an error code — untouched. The replacement is one cell
// wide (validated on load), so the cell keeps its width and no column shifts.
// A cell that starts with no known glyph is returned unchanged.
func replaceLeadingDot(cell, dot string) string {
	for _, g := range statusDots {
		if strings.HasPrefix(cell, g) {
			return dot + strings.TrimPrefix(cell, g)
		}
	}
	return cell
}

// isSingleCell reports whether s is exactly one terminal cell wide, which is what
// a configurable marker must be: pad, padLeft and truncate count runes, and the
// project deliberately carries no display-width dependency, so a two-cell glyph
// (an emoji, an ideograph) would silently shift every column to its right.
//
// It rejects anything that is not a single rune, anything non-printable, the
// zero-width combining marks, and the East Asian Wide/Fullwidth ranges.
func isSingleCell(s string) bool {
	r := []rune(s)
	if len(r) != 1 {
		return false
	}
	c := r[0]
	if !unicode.IsPrint(c) || unicode.IsSpace(c) {
		return false
	}
	if unicode.In(c, unicode.Mn, unicode.Me, unicode.Mc) {
		return false // combining marks occupy no cell of their own
	}
	return !isWide(c)
}

// wideRanges are the East Asian Wide/Fullwidth blocks plus the emoji planes —
// the code points a terminal renders two cells wide (Unicode TR11).
var wideRanges = []struct{ lo, hi rune }{
	{0x1100, 0x115F},   // Hangul Jamo initial consonants
	{0x2329, 0x232A},   // angle brackets
	{0x2E80, 0x303E},   // CJK radicals, Kangxi, CJK symbols
	{0x3041, 0x33FF},   // kana, Hangul compat jamo, CJK compatibility
	{0x3400, 0x4DBF},   // CJK extension A
	{0x4E00, 0x9FFF},   // CJK unified ideographs
	{0xA000, 0xA4CF},   // Yi
	{0xA960, 0xA97F},   // Hangul Jamo extended-A
	{0xAC00, 0xD7A3},   // Hangul syllables
	{0xF900, 0xFAFF},   // CJK compatibility ideographs
	{0xFE10, 0xFE19},   // vertical forms
	{0xFE30, 0xFE6F},   // CJK compatibility forms
	{0xFF00, 0xFF60},   // fullwidth forms
	{0xFFE0, 0xFFE6},   // fullwidth signs
	{0x1F300, 0x1F64F}, // emoji: symbols and pictographs, emoticons
	{0x1F680, 0x1F6FF}, // emoji: transport and map
	{0x1F900, 0x1F9FF}, // emoji: supplemental symbols
	{0x20000, 0x3FFFD}, // CJK extensions B and beyond
}

func isWide(c rune) bool {
	for _, r := range wideRanges {
		if c >= r.lo && c <= r.hi {
			return true
		}
	}
	return false
}
