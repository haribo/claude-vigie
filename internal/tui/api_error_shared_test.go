package tui

import (
	"encoding/json"
	"os"
	"testing"
)

// The web dashboard names an API error too, in its own implementation
// (internal/web/static/lib.js). architecture.md binds it to the TUI on content —
// "A divergence in content is debt, not design" — and two hand-kept lists of the
// same vocabulary is exactly how that debt is taken on. Both sides read this
// fixture, so a code added to one and forgotten in the other fails here (#584).
func TestAPIErrorLabelsMatchTheSharedFixture(t *testing.T) {
	body, err := os.ReadFile("../../test/fixtures/api-error-labels.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Code  int    `json:"code"`
			Label string `json:"label"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("the shared fixture has no cases — the extraction is broken, not the code")
	}
	for _, c := range fixture.Cases {
		if got := apiErrorLabel(c.Code); got != c.Label {
			t.Errorf("apiErrorLabel(%d) = %q, the shared fixture says %q", c.Code, got, c.Label)
		}
	}
}
