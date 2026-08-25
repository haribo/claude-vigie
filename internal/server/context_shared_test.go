package server

import (
	"encoding/json"
	"os"
	"testing"
)

// The context fill is derived here since ADR-0011 (#616): the daemon owns the
// model table and the rounding, and each client renders the answer. So this is
// where the shared fixture's `model` + `tokens` columns are proved, and
// internal/tui/column_shared_test.go and test/js/dashboard.test.mjs prove the
// `pct` → `want` half. One case list, one chain, three links.
//
// Before this, both clients derived the whole thing and the fixture checked that
// two implementations agreed. The rounding note it used to carry — Go rounding
// half to even where Math.round rounds half up — is gone with the second
// rounding.
func TestContextPctAgreesWithTheSharedFixture(t *testing.T) {
	b, err := os.ReadFile("../../test/fixtures/column-cases.json")
	if err != nil {
		t.Fatalf("reading the shared fixture: %v", err)
	}
	var f struct {
		Context []struct {
			Why    string `json:"why"`
			Model  string `json:"model"`
			Tokens *int64 `json:"tokens"`
			Pct    *int   `json:"pct"`
		} `json:"context"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("parsing the shared fixture: %v", err)
	}
	if len(f.Context) == 0 {
		t.Fatal("the shared fixture has no context cases — the extraction is broken, not the code")
	}

	for _, c := range f.Context {
		// The store keeps the reading and a flag for whether there is one at all,
		// which is the distinction #367 exists for; the fixture spells it as a null.
		var tokens int64
		known := c.Tokens != nil
		if known {
			tokens = *c.Tokens
		}
		got := contextPctView(tokens, known, c.Model)
		switch {
		case c.Pct == nil && got != nil:
			t.Errorf("contextPctView(model=%q, tokens=%v) = %d, want no reading — %s", c.Model, c.Tokens, *got, c.Why)
		case c.Pct != nil && got == nil:
			t.Errorf("contextPctView(model=%q, tokens=%v) = no reading, want %d — %s", c.Model, c.Tokens, *c.Pct, c.Why)
		case c.Pct != nil && *got != *c.Pct:
			t.Errorf("contextPctView(model=%q, tokens=%v) = %d, want %d — %s", c.Model, c.Tokens, *got, *c.Pct, c.Why)
		}
	}
}
