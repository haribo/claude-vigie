package tui

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// The order the attention queue is served in is a rule two clients now state:
// `n` in the terminal (#261) and the jump the dashboard grew with #667. A queue
// served in two different orders is worse than one window not having a queue at
// all, so the cases live in test/fixtures/attention-order-cases.json and both
// suites read them.
//
// This is ADR-0011's second family: what depends on the operator stays
// client-side and is proved against a shared case list, rather than moved to the
// server or hand-copied a third time.
type attentionOrderFixture struct {
	Cases []struct {
		Name     string            `json:"name"`
		Sessions []api.SessionView `json:"sessions"`
		Want     string            `json:"want"`
	} `json:"cases"`
}

func TestNextAttentionMatchesTheSharedFixture(t *testing.T) {
	f := loadFixture[attentionOrderFixture](t, "attention-order-cases.json")
	if len(f.Cases) == 0 {
		t.Fatal("the shared fixture carries no cases")
	}
	for _, c := range f.Cases {
		if got := nextAttention(c.Sessions); got != c.Want {
			t.Errorf("%s: nextAttention = %q, the shared fixture says %q", c.Name, got, c.Want)
		}
	}
}
