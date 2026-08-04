package tui

import (
	"slices"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func colKeys(cols []column) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.key()
	}
	return out
}

func TestActiveColumnsDefault(t *testing.T) {
	if got := colKeys(activeColumns(nil, nil)); !slices.Equal(got, columnKeys()) {
		t.Errorf("empty layout should give the built-in default:\n got=%v\nwant=%v", got, columnKeys())
	}
}

// TestHideKeepsPosition is the #315 fix: hiding a column must not move it.
func TestHideKeepsPosition(t *testing.T) {
	order := columnKeys()
	before := slices.Index(colKeys(pickerColumns(order)), "machine")
	hidden := toggleColumn(nil, "machine")
	after := slices.Index(colKeys(pickerColumns(order)), "machine")
	if before != after {
		t.Errorf("hidden column moved in the picker: before=%d after=%d", before, after)
	}
	if slices.Contains(colKeys(activeColumns(order, hidden)), "machine") {
		t.Error("hidden column still rendered in the table")
	}
}

func TestToggleColumn(t *testing.T) {
	hidden := toggleColumn(nil, "machine")
	if !columnHidden(hidden, "machine") {
		t.Error("machine should be hidden after toggle")
	}
	if columnHidden(toggleColumn(hidden, "machine"), "machine") {
		t.Error("machine should be shown again after a second toggle")
	}
	if columnHidden(toggleColumn(nil, "name"), "name") {
		t.Error("name is mandatory and must never hide")
	}
}

func TestMoveColumn(t *testing.T) {
	order := []string{"name", "status", "machine"}
	if got := moveColumn(order, "machine", -1); slices.Index(fullOrder(got), "machine") != slices.Index(fullOrder(order), "machine")-1 {
		t.Errorf("move up did not shift machine: %v", got)
	}
	// Reorder works on a hidden column too (position lives in the order, not visibility).
	if got := moveColumn(order, "machine", -1); slices.Index(fullOrder(got), "machine") != 1 {
		t.Errorf("machine should land at index 1: %v", fullOrder(got))
	}
	// Moving the first column up is a no-op.
	if got := moveColumn(order, "name", -1); slices.Index(fullOrder(got), "name") != 0 {
		t.Errorf("moving the first column up must be a no-op")
	}
}

func TestFullOrderDropsUnknownAppendsMissing(t *testing.T) {
	got := fullOrder([]string{"status", "ghostcolumn", "name"}) // one bogus, only 2 real
	if slices.Contains(got, "ghostcolumn") {
		t.Errorf("unknown key not dropped: %v", got)
	}
	if len(got) != len(columns) {
		t.Errorf("fullOrder must list every column (%d), got %d", len(columns), len(got))
	}
	if got[0] != "status" || got[1] != "name" {
		t.Errorf("saved order not honored first: %v", got[:2])
	}
}

func TestColumnLayoutTOMLRoundTrip(t *testing.T) {
	p := defaultPrefs()
	p.columnOrder = []string{"name", "status", "machine"}
	p.columnHidden = []string{"branch", "effort"}
	var f prefsFile
	if err := toml.Unmarshal([]byte(renderPrefsTOML(p)), &f); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.ColumnOrder, p.columnOrder) || !slices.Equal(f.ColumnHidden, p.columnHidden) {
		t.Errorf("layout round-trip: order=%v hidden=%v", f.ColumnOrder, f.ColumnHidden)
	}
}
