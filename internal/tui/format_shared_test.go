package tui

import (
	"testing"
	"time"
)

// The two figures each client formats for itself — a token count and a relative
// age — against one case list the dashboard reads too (#619).
//
// The tokens half is why the list exists. Go printed 1250 tokens as "1.2k" and
// JavaScript as "1.3k", because `%.1f` rounds half to even and `toFixed` rounds
// half away from zero; Go kept a trailing ".0" that JavaScript trimmed. Both sides
// had a green test the whole time — each had picked cases where its own answer was
// right, and the case that separated them was in neither list. That is the failure
// a shared fixture exists to make impossible, and it is the same rounding split
// #616 removed from the context percentage.

type formatFixture struct {
	Tokens []struct {
		Why  string `json:"why"`
		N    int64  `json:"n"`
		Want string `json:"want"`
	} `json:"tokens"`
	Age []struct {
		Why  string `json:"why"`
		Now  string `json:"now"`
		Seen string `json:"seen"`
		Want string `json:"want"`
	} `json:"age"`
}

func loadFormatFixture(t *testing.T) formatFixture {
	t.Helper()
	f := loadFixture[formatFixture](t, "format-cases.json")
	if len(f.Tokens) == 0 || len(f.Age) == 0 {
		t.Fatal("the shared fixture is missing a section — the extraction is broken, not the code")
	}
	return f
}

func TestTokenFormattingAgreesWithTheSharedFixture(t *testing.T) {
	for _, c := range loadFormatFixture(t).Tokens {
		if got := humanizeTokens(c.N); got != c.Want {
			t.Errorf("humanizeTokens(%d) = %q, want %q — %s", c.N, got, c.Want, c.Why)
		}
	}
}

func TestRelativeAgeAgreesWithTheSharedFixture(t *testing.T) {
	for _, c := range loadFormatFixture(t).Age {
		now, err := time.Parse(time.RFC3339, c.Now)
		if err != nil {
			t.Fatalf("the fixture's `now` is not a timestamp: %q", c.Now)
		}
		if got := relativeAge(c.Seen, now); got != c.Want {
			t.Errorf("relativeAge(%q) = %q, want %q — %s", c.Seen, got, c.Want, c.Why)
		}
	}
}
