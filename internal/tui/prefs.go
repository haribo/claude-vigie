package tui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/haribo/claude-vigie/internal/api"
)

// prefs holds the operator's TUI display preferences.
type prefs struct {
	hideEnded     bool          // hide ended (closed) sessions
	idleHideAfter time.Duration // hide sessions inactive longer than this; 0 = never
	sortKey       sortKey       // sessions table order
	sortReversed  bool          // reverse the sort direction
	groupBy       groupBy       // grouping of the sessions table
	notify        bool          // desktop notifications on working→attention (#260)
	columnOrder   []string      // display order of ALL table columns; empty = built-in default (#308)
	columnHidden  []string      // hidden table columns (#315)
	blink         bool          // animate the marker of a calling session (#389)
	// loadFailed says the preferences file exists but could not be used — it was
	// unreadable, empty, or not valid TOML. It is not a preference: it is a latch
	// that stops savePrefs stamping defaults over a file whose contents are still
	// there, which is how a corrupt file used to become an unrecoverable one
	// (#480). Empty means the file loaded, or genuinely did not exist yet.
	loadFailed string

	callMarker string // glyph for a calling session's dot; "" = defaultCallMarker
}

// defaultPrefs are used when no preferences file exists: hide only ended
// sessions, keep idle ones visible regardless of age, sort by last seen, notify.
func defaultPrefs() prefs {
	return prefs{
		hideEnded: true, idleHideAfter: 0, sortKey: sortLastSeen, groupBy: groupNone,
		notify: true, blink: true, callMarker: defaultCallMarker,
	}
}

// visible reports whether a session should be shown given these preferences.
func (p prefs) visible(s api.SessionView, now time.Time) bool {
	if p.hideEnded && s.Status == "ended" {
		return false
	}
	if p.idleHideAfter > 0 {
		if t, err := time.Parse(time.RFC3339, s.LastSeenAt); err == nil {
			return now.Sub(t) <= p.idleHideAfter
		}
	}
	return true
}

// prefsFile is the on-disk (TOML) shape of the preferences.
type prefsFile struct {
	HideEnded     bool     `toml:"hide_ended"`
	IdleHideAfter string   `toml:"idle_hide_after"` // Go duration; "" = never
	SortKey       string   `toml:"sort_key"`        // stable name (see sortNames); "" = default
	SortReversed  bool     `toml:"sort_reversed"`
	GroupBy       string   `toml:"group_by"`      // stable name (see groupNames); "" = off
	Notify        *bool    `toml:"notify"`        // pointer: absent = default (on)
	ColumnOrder   []string `toml:"column_order"`  // display order of all columns; empty = default (#308)
	ColumnHidden  []string `toml:"column_hidden"` // hidden columns (#315)
	Blink         *bool    `toml:"blink"`         // pointer: absent = default (on) (#389)
	CallMarker    string   `toml:"call_marker"`   // glyph for a calling session's dot (#389)
}

func prefsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vigie", "tui.toml"), nil
}

// renderPrefsTOML renders a commented TOML file for the given preferences, so
// edits (from the Settings tab) keep the file readable rather than a bare dump.
func renderPrefsTOML(p prefs) string {
	idle := ""
	if p.idleHideAfter > 0 {
		idle = p.idleHideAfter.String()
	}
	list := func(ks []string) string {
		if len(ks) == 0 {
			return "[]"
		}
		q := make([]string, len(ks))
		for i, k := range ks {
			q[i] = fmt.Sprintf("%q", k)
		}
		return "[" + strings.Join(q, ", ") + "]"
	}
	return fmt.Sprintf(`# vigie TUI preferences

# Desktop notifications when a session starts waiting on you (working→attention).
notify = %t

# Hide ended (closed) sessions from the list.
hide_ended = %t

# Hide sessions inactive for longer than this (Go duration, e.g. "30m", "2h").
# Empty = never hide by inactivity.
idle_hide_after = %q

# Sessions table order: last seen, tokens, status, name, rc.
sort_key = %q

# Reverse the sort direction.
sort_reversed = %t

# Group the sessions table: off, machine, project.
group_by = %q

# Sessions table columns: the display order of all columns, and which are hidden.
# Empty order = the built-in default. Edit from the Settings tab (space toggles
# visibility, [ ] reorder).
column_order = %s
column_hidden = %s

# Blink the status dot of a session that has called you (vigie call). Off leaves
# the call readable only in the DETAIL column.
blink = %t

# Glyph for a calling session's dot. Must be exactly one terminal cell wide: a
# two-cell glyph (an emoji, an ideograph) would shift every column to its right,
# so anything wider is ignored and the default is kept.
call_marker = %q
`, p.notify, p.hideEnded, idle, sortNames[p.sortKey], p.sortReversed, groupNames[p.groupBy], list(p.columnOrder), list(p.columnHidden), p.blink, p.callMarker)
}

// savePrefs writes the preferences file (best-effort; the UI must not block).
//
// Two rules, both learned from losing a layout (#480):
//
// It refuses to write when the file could not be read. Otherwise the first
// preference keystroke after a corrupt read would replace the operator's settings
// with the defaults the TUI fell back to — turning a recoverable file into a lost
// one, silently.
//
// And it writes through a temp file in the same directory, renamed over the
// target, so a TUI killed mid-write leaves the previous file intact rather than a
// truncated one. `internal/install` already did this for settings.json; the
// preferences were the asymmetry.
func savePrefs(p prefs) {
	if p.loadFailed != "" {
		return
	}
	path, err := prefsPath()
	if err != nil {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".tui-*.toml")
	if err != nil {
		return
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op once renamed
	if _, err := tmp.WriteString(renderPrefsTOML(p)); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(tmp.Name(), path)
}

// idlePresets are the selectable values for idle_hide_after (0 = never hide).
var idlePresets = []time.Duration{
	0, 15 * time.Minute, 30 * time.Minute, time.Hour, 3 * time.Hour, 6 * time.Hour,
}

// retentionPresets are the selectable server session-retention windows (0 = keep all).
var retentionPresets = []time.Duration{
	0, 24 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour,
}

// cycleDuration returns the next/previous preset relative to cur (wrapping).
func cycleDuration(presets []time.Duration, cur time.Duration, dir int) time.Duration {
	i := 0
	for j, d := range presets {
		if d == cur {
			i = j
			break
		}
	}
	i = (i + dir + len(presets)) % len(presets)
	return presets[i]
}

func cyclePreset(cur time.Duration, dir int) time.Duration {
	return cycleDuration(idlePresets, cur, dir)
}

func cycleRetention(cur time.Duration, dir int) time.Duration {
	return cycleDuration(retentionPresets, cur, dir)
}

// loadPrefs reads the preferences file, creating a commented default on first
// run. Any error falls back to the defaults: preferences must never block the UI.
func loadPrefs() prefs {
	p := defaultPrefs()
	path, err := prefsPath()
	if err != nil {
		return p
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Read-fallback: honor a tui.toml written under the pre-rename
		// ~/.config/claude-fleet directory; otherwise create a commented default.
		if dir, dErr := os.UserConfigDir(); dErr == nil {
			if d2, e2 := os.ReadFile(filepath.Join(dir, "claude-fleet", "tui.toml")); e2 == nil {
				data = d2
			}
		}
		if data == nil {
			savePrefs(p) // create a commented default file
			return p
		}
	} else if err != nil {
		// Keep the file and say so. Returning defaults silently was half the
		// defect: the next keystroke wrote them over whatever was still there.
		p.loadFailed = fmt.Sprintf("cannot read %s: %v", path, err)
		return p
	}
	// An empty file parses cleanly into zero values, which is worse than a parse
	// error: hide_ended flips to false and the column layout is lost, with nothing
	// to distinguish it from a deliberate configuration (#480).
	if len(bytes.TrimSpace(data)) == 0 {
		p.loadFailed = fmt.Sprintf("%s is empty", path)
		return p
	}
	var f prefsFile
	if err := toml.Unmarshal(data, &f); err != nil {
		p.loadFailed = fmt.Sprintf("cannot parse %s: %v", path, err)
		return p
	}
	p.hideEnded = f.HideEnded
	if d, err := time.ParseDuration(f.IdleHideAfter); err == nil {
		p.idleHideAfter = d
	}
	p.sortKey = sortKeyByName(f.SortKey)
	p.sortReversed = f.SortReversed
	p.groupBy = groupByName(f.GroupBy)
	if f.Notify != nil { // absent keeps the default (on)
		p.notify = *f.Notify
	}
	// A layout saved before a column rename keeps its old key (#393).
	p.columnOrder = migrateColumnKeys(f.ColumnOrder)
	p.columnHidden = migrateColumnKeys(f.ColumnHidden)
	if f.Blink != nil { // absent keeps the default (on)
		p.blink = *f.Blink
	}
	// A marker wider than one cell would shift every column to its right, since
	// the table pads by rune count and vigie carries no display-width dependency.
	// An invalid value is ignored rather than obeyed (#389).
	if isSingleCell(f.CallMarker) {
		p.callMarker = f.CallMarker
	}
	return p
}
