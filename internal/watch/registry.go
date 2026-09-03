package watch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/haribo/claude-vigie/internal/presence"
)

// sessionRecord is one entry of Claude Code's native session registry
// (~/.claude/sessions/<pid>.json), which Claude rewrites as a session runs. It is
// read-only, like everything here (ADR-0005). See issue #254. Only the fields we
// use are kept; the schema is Claude's private, undocumented format.
type sessionRecord struct {
	SessionID  string
	Status     string // busy | shell | idle | waiting (Claude's own enum)
	WaitingFor string // reason when Status == "waiting" (a permission/dialog)
	PID        int
	ProcStart  uint64 // /proc start time (clock ticks) — a pid-reuse guard
	// BridgeSessionID is Claude's remote-control session id (session_01…), set
	// while /rc is active; empty otherwise. It forms the resume URL (#253).
	BridgeSessionID string
}

// remoteURL builds the /rc resume URL from the bridge session id, or "" when /rc
// is off. The prefix matches the url field Claude writes verbatim in the
// transcript's bridge_status lines.
func (r sessionRecord) remoteURL() string {
	if r.BridgeSessionID == "" {
		return ""
	}
	return "https://claude.ai/code/" + r.BridgeSessionID
}

// readRegistry reads Claude Code's session registry and returns the record per
// sessionId. Missing/older clients simply yield an empty map, so callers must
// treat it as best-effort and fall back to the other observers.
func readRegistry() map[string]sessionRecord {
	m := map[string]sessionRecord{}
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
		var raw struct {
			SessionID       string `json:"sessionId"`
			Status          string `json:"status"`
			WaitingFor      string `json:"waitingFor"`
			PID             int    `json:"pid"`
			ProcStart       string `json:"procStart"` // Claude stores it as a string
			BridgeSessionID string `json:"bridgeSessionId"`
		}
		if err := json.Unmarshal(data, &raw); err != nil || raw.SessionID == "" {
			continue
		}
		ps, _ := strconv.ParseUint(raw.ProcStart, 10, 64)
		m[raw.SessionID] = sessionRecord{
			SessionID: raw.SessionID, Status: raw.Status, WaitingFor: raw.WaitingFor,
			PID: raw.PID, ProcStart: ps, BridgeSessionID: raw.BridgeSessionID,
		}
	}
	return m
}

// remoteControlled returns, per sessionId, whether the session carries a
// bridgeSessionId (i.e. /rc is active) — read from the same registry.
func remoteControlled() map[string]bool {
	m := map[string]bool{}
	for id, rec := range readRegistry() {
		m[id] = rec.BridgeSessionID != ""
	}
	return m
}

// registryDead reports whether the record's backing process is *confidently* gone
// — we have a pid + start time to check and it no longer matches (a crash or a
// missed SessionEnd). A record without that info returns false (unknown), so a
// schema drift never produces a false "ended". Uses the same pid-reuse guard as
// the presence package.
func registryDead(rec sessionRecord) bool {
	if rec.PID <= 0 || rec.ProcStart == 0 {
		return false
	}
	// Gone, not "not Live". An unreadable /proc — hidepid, a namespace that does
	// not expose the pid — used to answer the same as an absent one, and this
	// short-circuits everything: every session Claude Code listed read `ended` on
	// the next scan (#663). The uncertainty arrives through the value here rather
	// than through a missing record, and it deserves the same answer: not dead.
	return presence.Status(presence.Mapping{PID: rec.PID, StartTime: rec.ProcStart}) == presence.Gone
}

// mapRegistryStatus maps Claude's registry status onto a fleet status. "shell"
// (the user dropped to a bash shell inside Claude) reads idle — the session is
// alive but Claude is not producing. An unknown value degrades to idle: a live
// session is never a false "ended".
func mapRegistryStatus(s string) string {
	switch s {
	case "waiting":
		return "waiting"
	case "busy":
		return "working"
	case "idle", "shell":
		return "idle"
	default:
		return "idle"
	}
}

// capText truncates s to at most n runes, appending an ellipsis when it cuts.
func capText(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
