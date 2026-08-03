// Package install merges claude-fleet reporting hooks into the user's Claude
// Code settings (~/.claude/settings.json), idempotently, preserving any
// existing hooks and settings. Client side; imports nothing heavy.
//
// A hook "leg" is identified by its config-path override: the production leg has
// none, a dev leg carries VIGIE_CONFIG=<path> (or the deprecated FLEET_CONFIG=<path>,
// still recognized). Legs are independent, so a session can report to several
// servers at once and each leg is installed or removed without touching the others.
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

// reportMarker identifies a claude-fleet reporting hook, independent of the
// binary path.
const reportMarker = "report --event="

// SettingsPath returns the path to the user's Claude Code settings file.
func SettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// command builds the report hook command for one event and leg. configPath is
// the config-path override for the leg ("" for the production leg); a dev leg
// carries it as VIGIE_CONFIG.
func command(binPath, configPath, event string) string {
	if configPath == "" {
		return fmt.Sprintf("%s report --event=%s", binPath, event)
	}
	return fmt.Sprintf("VIGIE_CONFIG=%s %s report --event=%s", configPath, binPath, event)
}

// legMarkers are the env-var prefixes that tag a dev leg: the current VIGIE_CONFIG
// and the deprecated FLEET_CONFIG, still recognized so legs installed before the
// rename keep matching (#289).
var legMarkers = []string{"VIGIE_CONFIG=", "FLEET_CONFIG="}

// owns reports whether a hook command belongs to the leg identified by configPath
// (production when empty). A dev leg is recognized under either env-var name, so
// a leg installed before the rename is still uninstalled or replaced correctly.
func owns(cmd, configPath string) bool {
	if !strings.Contains(cmd, reportMarker) {
		return false
	}
	if configPath == "" {
		for _, m := range legMarkers {
			if strings.Contains(cmd, m) {
				return false
			}
		}
		return true
	}
	for _, m := range legMarkers {
		if strings.Contains(cmd, m+configPath) {
			return true
		}
	}
	return false
}

// Install merges the reporting hooks for one leg (binPath, configPath) into the
// settings file and returns the path written. Other legs are left untouched.
func Install(events []string, binPath, configPath string, timeout int) (string, error) {
	path, err := SettingsPath()
	if err != nil {
		return "", err
	}
	existing, err := readSettings(path)
	if err != nil {
		return "", err
	}
	merged, err := mergeHooks(existing, events, binPath, configPath, timeout)
	if err != nil {
		return "", err
	}
	return path, writeSettings(path, merged)
}

// Uninstall removes the hooks for one leg (configPath; production when empty),
// leaving other legs and foreign hooks in place.
func Uninstall(configPath string) (string, error) {
	path, err := SettingsPath()
	if err != nil {
		return "", err
	}
	existing, err := readSettings(path)
	if err != nil {
		return "", err
	}
	cleaned, err := removeHooks(existing, configPath)
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

func mergeHooks(existing []byte, events []string, binPath, configPath string, timeout int) ([]byte, error) {
	s, hooks, err := parseSettings(existing)
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		hooks[ev] = append(stripLeg(hooks[ev], configPath), hookMatcher{
			Matcher: "",
			Hooks: []hookCommand{{
				Type:    "command",
				Command: command(binPath, configPath, ev),
				Timeout: timeout,
			}},
		})
	}
	return encodeSettings(s, hooks)
}

func removeHooks(existing []byte, configPath string) ([]byte, error) {
	s, hooks, err := parseSettings(existing)
	if err != nil {
		return nil, err
	}
	for ev, matchers := range hooks {
		kept := stripLeg(matchers, configPath)
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

func stripLeg(matchers []hookMatcher, configPath string) []hookMatcher {
	kept := make([]hookMatcher, 0, len(matchers))
	for _, m := range matchers {
		if !matcherIsLeg(m, configPath) {
			kept = append(kept, m)
		}
	}
	return kept
}

func matcherIsLeg(m hookMatcher, configPath string) bool {
	for _, h := range m.Hooks {
		if owns(h.Command, configPath) {
			return true
		}
	}
	return false
}
