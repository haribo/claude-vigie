package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/haribo/claude-fleet/internal/api"
)

// prefs holds the operator's TUI display preferences.
type prefs struct {
	hideEnded     bool          // hide ended (closed) sessions
	idleHideAfter time.Duration // hide sessions inactive longer than this; 0 = never
}

// defaultPrefs are used when no preferences file exists: hide only ended
// sessions, keep idle ones visible regardless of age.
func defaultPrefs() prefs {
	return prefs{hideEnded: true, idleHideAfter: 0}
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
	HideEnded     bool   `toml:"hide_ended"`
	IdleHideAfter string `toml:"idle_hide_after"` // Go duration; "" = never
}

func prefsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "claude-fleet", "tui.toml"), nil
}

// renderPrefsTOML renders a commented TOML file for the given preferences, so
// edits (from the Settings tab) keep the file readable rather than a bare dump.
func renderPrefsTOML(p prefs) string {
	idle := ""
	if p.idleHideAfter > 0 {
		idle = p.idleHideAfter.String()
	}
	return fmt.Sprintf(`# claude-fleet TUI preferences

# Hide ended (closed) sessions from the list.
hide_ended = %t

# Hide sessions inactive for longer than this (Go duration, e.g. "30m", "2h").
# Empty = never hide by inactivity.
idle_hide_after = %q
`, p.hideEnded, idle)
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

// idlePresets are the selectable values for idle_hide_after in the Settings tab
// (0 = never hide by inactivity).
var idlePresets = []time.Duration{
	0, 15 * time.Minute, 30 * time.Minute, time.Hour, 3 * time.Hour, 6 * time.Hour,
}

// cyclePreset returns the next/previous idle preset relative to cur.
func cyclePreset(cur time.Duration, dir int) time.Duration {
	i := 0
	for j, d := range idlePresets {
		if d == cur {
			i = j
			break
		}
	}
	i = (i + dir + len(idlePresets)) % len(idlePresets)
	return idlePresets[i]
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
		savePrefs(p) // create a commented default file
		return p
	}
	if err != nil {
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
	return p
}
