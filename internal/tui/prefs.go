package tui

import (
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
	columnOrder   []string      // visible table columns, in order; empty = built-in default (#308)
}

// defaultPrefs are used when no preferences file exists: hide only ended
// sessions, keep idle ones visible regardless of age, sort by last seen, notify.
func defaultPrefs() prefs {
	return prefs{hideEnded: true, idleHideAfter: 0, sortKey: sortLastSeen, groupBy: groupNone, notify: true}
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
	GroupBy       string   `toml:"group_by"`     // stable name (see groupNames); "" = off
	Notify        *bool    `toml:"notify"`       // pointer: absent = default (on)
	ColumnOrder   []string `toml:"column_order"` // visible table columns in order; empty = default (#308)
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
	cols := "[]"
	if len(p.columnOrder) > 0 {
		quoted := make([]string, len(p.columnOrder))
		for i, k := range p.columnOrder {
			quoted[i] = fmt.Sprintf("%q", k)
		}
		cols = "[" + strings.Join(quoted, ", ") + "]"
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

# Sessions table columns: the visible columns, in display order. Empty = the
# built-in default. Edit from the Settings tab (space toggles, [ ] reorder).
column_order = %s
`, p.notify, p.hideEnded, idle, sortNames[p.sortKey], p.sortReversed, groupNames[p.groupBy], cols)
}

// savePrefs writes the preferences file (best-effort; the UI must not block).
func savePrefs(p prefs) {
	path, err := prefsPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(renderPrefsTOML(p)), 0o600)
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
		return p
	}
	var f prefsFile
	if err := toml.Unmarshal(data, &f); err != nil {
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
	p.columnOrder = f.ColumnOrder
	return p
}
