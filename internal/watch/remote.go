package watch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// remoteControlled reads Claude Code's session files and returns, per sessionId,
// whether the session is remotely controlled — i.e. it carries a non-empty
// bridgeSessionId, which Claude sets when /rc is active. This is the read-only
// detection described in docs/design/remote-control.md (ADR-0005: observe-only).
func remoteControlled() map[string]bool {
	m := map[string]bool{}
	home, err := os.UserHomeDir()
	if err != nil {
		return m
	}
	dir := filepath.Join(home, ".claude", "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return m
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // confined to ~/.claude/sessions
		if err != nil {
			continue
		}
		var s struct {
			SessionID       string `json:"sessionId"`
			BridgeSessionID string `json:"bridgeSessionId"`
		}
		if err := json.Unmarshal(data, &s); err != nil || s.SessionID == "" {
			continue
		}
		m[s.SessionID] = s.BridgeSessionID != ""
	}
	return m
}
