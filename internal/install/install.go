// Package install merges claude-fleet reporting hooks into the user's Claude
// Code settings (~/.claude/settings.json), idempotently, preserving any
// existing hooks and settings. Client side; imports nothing heavy.
package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

type hookMatcher struct {
	Matcher string        `json:"matcher"`
	Hooks   []hookCommand `json:"hooks"`
}

// ourMarker identifies hooks this tool manages, so re-running init replaces
// them instead of duplicating and uninstall removes only ours.
const ourMarker = "claude-fleet report"

// SettingsPath returns the path to the user's Claude Code settings file.
func SettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// Install merges reporting hooks for the given events (each invoking
// "<binPath> report --event=<event>") into the settings file, and returns the
// path written.
func Install(events []string, binPath string, timeout int) (string, error) {
	path, err := SettingsPath()
	if err != nil {
		return "", err
	}
	existing, err := readSettings(path)
	if err != nil {
		return "", err
	}
	merged, err := mergeHooks(existing, events, binPath, timeout)
	if err != nil {
		return "", err
	}
	return path, writeSettings(path, merged)
}

// Uninstall removes only claude-fleet hooks from the settings file.
func Uninstall() (string, error) {
	path, err := SettingsPath()
	if err != nil {
		return "", err
	}
	existing, err := readSettings(path)
	if err != nil {
		return "", err
	}
	cleaned, err := removeHooks(existing)
	if err != nil {
		return "", err
	}
	return path, writeSettings(path, cleaned)
}

func readSettings(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}

func writeSettings(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("creating settings dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func mergeHooks(existing []byte, events []string, binPath string, timeout int) ([]byte, error) {
	s, hooks, err := parseSettings(existing)
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		hooks[ev] = append(stripOurs(hooks[ev]), hookMatcher{
			Matcher: "",
			Hooks: []hookCommand{{
				Type:    "command",
				Command: fmt.Sprintf("%s report --event=%s", binPath, ev),
				Timeout: timeout,
			}},
		})
	}
	return encodeSettings(s, hooks)
}

func removeHooks(existing []byte) ([]byte, error) {
	s, hooks, err := parseSettings(existing)
	if err != nil {
		return nil, err
	}
	for ev, matchers := range hooks {
		kept := stripOurs(matchers)
		if len(kept) == 0 {
			delete(hooks, ev)
		} else {
			hooks[ev] = kept
		}
	}
	return encodeSettings(s, hooks)
}

func parseSettings(existing []byte) (map[string]json.RawMessage, map[string][]hookMatcher, error) {
	s := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &s); err != nil {
			return nil, nil, fmt.Errorf("parsing settings: %w", err)
		}
	}
	hooks := map[string][]hookMatcher{}
	if raw, ok := s["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, nil, fmt.Errorf("parsing hooks: %w", err)
		}
	}
	return s, hooks, nil
}

func encodeSettings(s map[string]json.RawMessage, hooks map[string][]hookMatcher) ([]byte, error) {
	if len(hooks) == 0 {
		delete(s, "hooks")
	} else {
		raw, err := json.Marshal(hooks)
		if err != nil {
			return nil, fmt.Errorf("encoding hooks: %w", err)
		}
		s["hooks"] = raw
	}
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding settings: %w", err)
	}
	return append(out, '\n'), nil
}

func stripOurs(matchers []hookMatcher) []hookMatcher {
	kept := make([]hookMatcher, 0, len(matchers))
	for _, m := range matchers {
		if !matcherIsOurs(m) {
			kept = append(kept, m)
		}
	}
	return kept
}

func matcherIsOurs(m hookMatcher) bool {
	for _, h := range m.Hooks {
		if strings.Contains(h.Command, ourMarker) {
			return true
		}
	}
	return false
}
