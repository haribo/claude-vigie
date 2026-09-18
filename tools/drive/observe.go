//go:build linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/apiclient"
	"github.com/haribo/claude-vigie/internal/config"
)

// observation is one row of the table this tool exists to print: what Claude Code
// says about a session, next to what vigie makes of it.
type observation struct {
	Elapsed  int    `json:"elapsed_seconds"`
	Registry string `json:"registry"` // Claude Code's own word: busy | shell | idle | waiting
	Status   string `json:"vigie"`    // what the board shows
	Detail   string `json:"detail"`
}

// registryStatus reads Claude Code's own session record straight from disk.
//
// It deliberately does **not** call into `internal/watch`. An instrument that
// shares code with the thing it measures reports the rule rather than the reality,
// which is the failure this harness exists to end: ADR-0015's residual was wrong
// three times behind a measurement that read through the same parsing gap as the
// code (#818, #843). Here the only thing between the file and the table is
// `encoding/json`.
func registryStatus(home, sessionID string) string {
	matches, err := filepath.Glob(filepath.Join(home, ".claude", "sessions", "*.json"))
	if err != nil {
		return ""
	}
	for _, m := range matches {
		data, err := os.ReadFile(m) //nolint:gosec // confined to the registry dir
		if err != nil {
			continue
		}
		var rec struct {
			SessionID string `json:"sessionId"`
			Status    string `json:"status"`
		}
		if json.Unmarshal(data, &rec) == nil && rec.SessionID == sessionID {
			return rec.Status
		}
	}
	return ""
}

// newSessionID returns the session id of the record that appeared since `before`,
// which is how the harness learns the id of the session it just launched: Claude
// Code names it, nobody else can.
func newSessionID(home string, before map[string]bool) string {
	matches, err := filepath.Glob(filepath.Join(home, ".claude", "sessions", "*.json"))
	if err != nil {
		return ""
	}
	sort.Strings(matches)
	for _, m := range matches {
		if before[m] {
			continue
		}
		data, err := os.ReadFile(m) //nolint:gosec // confined to the registry dir
		if err != nil {
			continue
		}
		var rec struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(data, &rec) == nil && rec.SessionID != "" {
			return rec.SessionID
		}
	}
	return ""
}

// registryFiles is the set to compare against after a launch.
func registryFiles(home string) map[string]bool {
	out := map[string]bool{}
	matches, err := filepath.Glob(filepath.Join(home, ".claude", "sessions", "*.json"))
	if err != nil {
		return out
	}
	for _, m := range matches {
		out[m] = true
	}
	return out
}

// boardStatus asks the daemon what it shows for a session, through the same
// endpoint and the same client every other reader uses. Unlike the registry side,
// reading this through vigie's own client is the point: the question is what the
// operator sees.
func boardStatus(cfg *config.Config, sessionID string) (status, detail string) {
	views, err := apiclient.Get[[]api.SessionView](cfg, "/api/sessions", "sessions")
	if err != nil {
		return "unreachable", ""
	}
	for _, v := range views {
		if v.ID == sessionID {
			return v.Status, v.DetailText
		}
	}
	return "absent", ""
}
