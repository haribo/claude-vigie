package watch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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
			SessionID  string `json:"sessionId"`
			Status     string `json:"status"`
			WaitingFor string `json:"waitingFor"`
			PID        int    `json:"pid"`
			ProcStart  string `json:"procStart"` // Claude stores it as a string
		}
		if err := json.Unmarshal(data, &raw); err != nil || raw.SessionID == "" {
			continue
		}
		ps, _ := strconv.ParseUint(raw.ProcStart, 10, 64)
		m[raw.SessionID] = sessionRecord{
			SessionID: raw.SessionID, Status: raw.Status, WaitingFor: raw.WaitingFor,
			PID: raw.PID, ProcStart: ps,
		}
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
// registryStatuses is Claude Code's own status vocabulary, mapped onto vigie's.
//
// A map rather than a switch, because the mapping and the "does this build know
// the word" check must never be two lists. Two places that have to agree, with
// one of them updated, is precisely how #816 was created — a new distinction
// taught to `reconcileWatch` and not to `holdsWaiting` (#821). One list cannot
// disagree with itself.
var registryStatuses = map[string]string{
	"waiting": "waiting",
	"busy":    "working",
	"idle":    "idle",
	"shell":   "idle", // a `!` prompt: alive and producing nothing (#280)
}

// mapRegistryStatus maps a registry status onto a vigie status, reading an
// unrecognized word as `idle`.
//
// `idle` is the right *display* answer for a word we cannot interpret: it claims
// the least. Saying it silently is not — see unknownRegistryStatuses.
func mapRegistryStatus(s string) string {
	if v, ok := registryStatuses[s]; ok {
		return v
	}
	return "idle"
}

// unknownRegistryStatuses returns, sorted, the status words in reg that this
// build does not know.
//
// The registry schema is Claude Code's private, undocumented format, so a status
// renamed upstream would show every working session at rest, on every machine,
// and nothing anywhere would say why — the whole fleet wrong and the test suite
// green ([ADR-0016](../../docs/adr/0016-capture-claude-code-artefacts.md)).
//
// **No test can catch that.** A test only ever sees words we wrote down; the
// rename is by definition a word we have not. Only a watcher running against the
// real thing can notice, which is why this is instrumentation rather than an
// assertion — and why what the tests below guard is the mechanism, not the
// vocabulary.
func unknownRegistryStatuses(reg map[string]sessionRecord) []string {
	var unknown []string
	seen := map[string]bool{}
	for _, rec := range reg {
		if _, ok := registryStatuses[rec.Status]; ok || seen[rec.Status] {
			continue
		}
		seen[rec.Status] = true
		unknown = append(unknown, rec.Status)
	}
	sort.Strings(unknown)
	return unknown
}

// capText truncates s to at most n runes, appending an ellipsis when it cuts.
func capText(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
