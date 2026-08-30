package tui

import (
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// What the sessions table opens on, and the order each key produces. The dashboard
// reads the same list (#645).
//
// The lists built for #619 prove the rules agree — the filter, the grouping, the
// formatting — and none of them says which key the table opens on or in which
// direction. Three columns had drifted apart unnoticed for exactly that reason.

type sortFixture struct {
	Now     string               `json:"now"`
	Default struct{ Key string } `json:"default"`
	Fleet   []api.SessionView    `json:"fleet"`
	Cases   []struct {
		Key  string   `json:"key"`
		Why  string   `json:"why"`
		Want []string `json:"want"`
	} `json:"cases"`
}

// The two clients store their sort preference under different names: the TUI's
// are prose ("last seen"), the dashboard's are column keys ("seen"). Each name is
// private to that client's own storage, so neither is wrong — but a case list has
// to pick one, and it picks the column key. Renaming a key on either side fails
// here rather than silently resetting an operator's saved preference.
var sortKeyForColumn = map[string]sortKey{
	"name":   sortName,
	"seen":   sortLastSeen,
	"total":  sortTokens,
	"status": sortStatus,
	"rc":     sortRC,
}

func TestTheTableOpensOnTheKeyTheSharedFixtureNames(t *testing.T) {
	f := loadFixture[sortFixture](t, "sort-cases.json")
	if f.Default.Key == "" {
		t.Fatal("the shared fixture names no default key — the extraction is broken, not the code")
	}
	want, ok := sortKeyForColumn[f.Default.Key]
	if !ok {
		t.Fatalf("the fixture opens on a column %q this build does not sort by", f.Default.Key)
	}
	if got := defaultPrefs().sortKey; got != want {
		t.Errorf("the table opens on %q, the shared fixture says %q", sortNames[got], f.Default.Key)
	}
	if defaultPrefs().sortReversed {
		t.Error("the table opens reversed; the fixture's orders are the unreversed ones")
	}
}

func TestEverySortKeyAgreesWithTheSharedFixture(t *testing.T) {
	f := loadFixture[sortFixture](t, "sort-cases.json")
	if len(f.Cases) == 0 || len(f.Fleet) < 2 {
		t.Fatal("the shared fixture is missing a section — the extraction is broken, not the code")
	}
	for _, c := range f.Cases {
		key, ok := sortKeyForColumn[c.Key]
		if !ok {
			t.Fatalf("the fixture names a column %q this build does not sort by", c.Key)
		}
		got := make([]api.SessionView, len(f.Fleet))
		copy(got, f.Fleet)
		sortSessions(got, key, false)
		for i, want := range c.Want {
			if got[i].ID != want {
				t.Errorf("sorting by %q gave %s at position %d, want %s — %s",
					c.Key, got[i].ID, i, want, c.Why)
			}
		}
	}
}
