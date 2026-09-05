package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
)

// #727. The usage row read `just now old · cannot refresh`. The age and the word
// "old" were concatenated, and the under-a-minute case renders as a phrase — "just
// now" — not as a quantity. It shows the moment a refresh fails right after a
// successful fetch, because a failure forces the degraded row whatever the
// snapshot's age.
func TestTheUsageRowReadsAsASentenceAtEveryAge(t *testing.T) {
	for _, c := range []struct {
		age  time.Duration
		want string
	}{
		{10 * time.Second, "just now"},
		{90 * time.Second, "1m old"},
		{3 * time.Hour, "3h old"},
	} {
		got := agePhrase(c.age)
		if got != c.want {
			t.Errorf("at %s: %q, want %q", c.age, got, c.want)
		}
		if strings.Contains(got, "now old") {
			t.Errorf("at %s the row reads %q", c.age, got)
		}
	}
}

// The repro, through the row itself: a snapshot fetched seconds ago whose refresh
// then failed. The failure forces the degraded branch whatever the age, which is
// how the under-a-minute case reaches a line that assumes a quantity.
func TestTheUsageRowIsReadableRightAfterAFailedRefresh(t *testing.T) {
	m := failingModel(t, srcUsage)
	m = m.applyDataMsg(m.fetchUsageCmd()())
	m.usage = api.UsageReport{FetchedAt: m.now().Add(-10 * time.Second).UTC().Format(time.RFC3339)}

	got := m.usageRow().detail
	if !strings.HasPrefix(got, "just now ·") {
		t.Errorf("row = %q, want it to open with `just now ·`", got)
	}
	if !strings.Contains(got, "cannot refresh") {
		t.Errorf("row = %q, want it to say the refresh failed", got)
	}
}
