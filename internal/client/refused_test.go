package client

import (
	"strings"
	"testing"
	"time"
)

// ADR-0013 decision 2: the refusal is observable before it is strict.
//
// A hook always exits 0 — it must never fail the operator's session — so a report
// refused for not coming from Claude Code writes a count and nothing else. If
// nothing reads that count, the day the variable is renamed vigie loses the whole
// fleet and says nothing: an empty board that looks like a quiet one. That is
// #663's failure ("I could not look" read as "there is nothing"), one layer up.
//
// The notice is a pure function so it can be asserted without a terminal.
func TestTheRefusalNoticeTellsTheOperatorWhatToDo(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	if got := refusalNotice(0, time.Time{}, now); got != "" {
		t.Errorf("notice = %q with nothing refused, want silence", got)
	}

	got := refusalNotice(7, now.Add(-90*time.Second), now)
	if got == "" {
		t.Fatal("no notice for seven refused reports — the board is missing sessions and says nothing")
	}
	for _, want := range []string{"7", "ADR-0013"} {
		if !strings.Contains(got, want) {
			t.Errorf("notice %q does not carry %q", got, want)
		}
	}

	// How recent it is decides whether the operator acts now or shrugs: a refusal
	// from two weeks ago is history, one from a minute ago is a fleet going dark.
	recent := refusalNotice(1, now.Add(-30*time.Second), now)
	old := refusalNotice(1, now.Add(-14*24*time.Hour), now)
	if recent == old {
		t.Error("the notice reads the same a minute later and a fortnight later")
	}
}
