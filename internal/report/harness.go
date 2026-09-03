package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/haribo/claude-vigie/internal/clock"
)

// requireClaudeCode refuses a report that does not come from the Claude Code
// session it claims to describe (ADR-0013).
//
// Another CLI reads `~/.claude/settings.json` and runs the hooks it finds there,
// so it calls `vigie report` exactly as Claude Code does. Its sessions arrived as
// rows vigie could not name — the title comes from a transcript it does not
// write — could not end — liveness follows a `claude` process it does not run —
// and counted one row per subagent, because a subagent is only *not* a session
// while the harness gives it the parent's id. The fleet count stopped meaning
// anything (#709).
//
// The check is on the client because the server cannot make it: it receives JSON,
// and a `harness` field in it would be declared by the very party we are trying to
// identify.
//
// Presence of the variable proves a Claude-shaped environment. Equality with the
// payload proves the caller reports *its own* session rather than relaying
// someone else's.
//
// **The absent case refuses.** A foreign CLI sets nothing, so "absent → allow"
// would be no guard at all.
func requireClaudeCode(sessionID string) error {
	env := os.Getenv(sessionIDEnv) //nolint:forbidigo // Claude Code's own handle, not vigie config
	switch {
	case env == "":
		return fmt.Errorf("not posting: %s is unset — vigie reports Claude Code sessions (ADR-0013)", sessionIDEnv)
	case env != sessionID:
		return fmt.Errorf("not posting: %s is %q but the payload reports %q — a session may only report itself",
			sessionIDEnv, env, sessionID)
	}
	return nil
}

// refusals is the record a refused report leaves behind, so the operator can be
// told rather than left with a board that quietly stopped filling.
//
// A hook always exits 0 — it must never fail the operator's session — so a
// refusal that wrote nothing would be invisible. The day Claude Code renames or
// drops that variable, vigie would lose the whole fleet and say nothing: the
// failure #663 was about ("I could not look" read as "there is nothing"), one
// layer up. The count is what the TUI preflight surfaces.
type refusals struct {
	Count int    `json:"count"`
	Last  string `json:"last"`  // RFC3339
	First string `json:"first"` // RFC3339, so "since when" is answerable
}

// refusalPath is derived purely from HOME (not XDG_STATE_HOME) so the hook's
// environment and the TUI's resolve to the same file — the same rule as presence
// (ADR-0006), compaction and the unreachable mark.
func refusalPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "vigie", "refused-reports.json"), nil
}

// recordRefusal counts one refused report. Best-effort in both directions: it
// never returns an error to a hook, and a lost increment under two concurrent
// hooks costs the operator one unit of a number they read as "some" — worth less
// than the lock it would take to prevent.
func recordRefusal() {
	p, err := refusalPath()
	if err != nil {
		return
	}
	var r refusals
	if b, err := os.ReadFile(p); err == nil { //nolint:gosec // a path we derive ourselves
		_ = json.Unmarshal(b, &r)
	}
	now := clock.Now().UTC().Format(time.RFC3339)
	if r.First == "" {
		r.First = now
	}
	r.Count++
	r.Last = now
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(p, b, 0o600)
}

// RefusedReports returns how many reports were refused for not coming from Claude
// Code, and when the last one was. Zero and the zero time when there are none.
//
// Read by the TUI preflight: the refusal is meant to be observable before it is
// strict, so that a convention changing under us is announced rather than
// discovered as an empty board.
func RefusedReports() (int, time.Time) {
	p, err := refusalPath()
	if err != nil {
		return 0, time.Time{}
	}
	b, err := os.ReadFile(p) //nolint:gosec // a path we derive ourselves
	if err != nil {
		return 0, time.Time{}
	}
	var r refusals
	if json.Unmarshal(b, &r) != nil {
		return 0, time.Time{}
	}
	last, err := time.Parse(time.RFC3339, r.Last)
	if err != nil {
		return r.Count, time.Time{}
	}
	return r.Count, last
}

// ClearRefusedReports forgets the record. The preflight calls it once it has told
// the operator, so the next launch reports what happened since rather than a
// total that only grows.
func ClearRefusedReports() {
	if p, err := refusalPath(); err == nil {
		_ = os.Remove(p)
	}
}
