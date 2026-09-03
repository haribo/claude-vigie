package client

import (
	"fmt"
	"os"
	"time"

	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/report"
	"github.com/haribo/claude-vigie/internal/tui"
)

// refusalNotice is what the operator is told about reports refused for not
// coming from Claude Code (ADR-0013), or "" when there are none.
//
// It exists because the refusal is silent by construction: a hook always exits 0,
// so a refused report writes a count and disappears. Without this, the day Claude
// Code renames `CLAUDE_CODE_SESSION_ID` vigie stops seeing the fleet and shows an
// empty board — indistinguishable from a quiet one. That is #663's failure, one
// layer up, and decision 2 of the ADR is what keeps it from happening in silence.
//
// The age is in the sentence because it decides what the operator does: a refusal
// from a fortnight ago is history — another CLI ran once — while one from a minute
// ago is a fleet going dark right now.
func refusalNotice(n int, last, now time.Time) string {
	if n <= 0 {
		return ""
	}
	when := "at some point"
	if !last.IsZero() {
		when = "last " + tui.HumanizeDuration(now.Sub(last)) + " ago"
	}
	return fmt.Sprintf(
		"note: %d report(s) refused, %s — they did not come from a Claude Code session.\n"+
			"      Expected if another CLI runs the hooks in ~/.claude/settings.json.\n"+
			"      If these are your own Claude Code sessions, they are missing from the board (ADR-0013).",
		n, when)
}

// warnAboutRefusedReports prints the notice and forgets the record, so the next
// launch reports what happened since rather than a total that only grows.
//
// It never blocks: the preflight is strict about a daemon it cannot reach, and
// this is not that — it is a fact about hooks on this machine, and the operator
// decides what it means.
func warnAboutRefusedReports() {
	n, last := report.RefusedReports()
	notice := refusalNotice(n, last, clock.Now())
	if notice == "" {
		return
	}
	fmt.Fprintln(os.Stderr, notice)
	report.ClearRefusedReports()
}
