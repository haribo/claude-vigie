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
	got := colKeys(activeColumns(nil))
	if !slices.Equal(got, columnKeys()) {
		t.Errorf("empty order should give the built-in default:\n got=%v\nwant=%v", got, columnKeys())
	}
}

func TestActiveColumnsReorderAndHide(t *testing.T) {
	// A saved order both reorders and hides (only the listed keys, in that order).
	order := []string{"status", "name", "machine"}
	if got := colKeys(activeColumns(order)); !slices.Equal(got, order) {
		t.Errorf("active = %v, want %v", got, order)
	}
}

func TestToggleColumn(t *testing.T) {
	base := columnKeys()
	hidden := toggleColumn(base, "machine")
	if columnVisible(hidden, "machine") {
		t.Error("machine should be hidden after toggle")
	}
	if !columnVisible(toggleColumn(hidden, "machine"), "machine") {
		t.Error("machine should be visible again after a second toggle")
	}
	if !columnVisible(toggleColumn(base, "name"), "name") {
		t.Error("name is mandatory and must never hide")
	}
}

func TestMoveColumn(t *testing.T) {
	order := []string{"name", "status", "machine"}
	if got := moveColumn(order, "machine", -1); !slices.Equal(got, []string{"name", "machine", "status"}) {
		t.Errorf("move up = %v", got)
	}
	if got := moveColumn(order, "name", -1); !slices.Equal(got, order) {
		t.Errorf("moving the first column up must be a no-op, got %v", got)
	}
}

func TestEffectiveOrderDropsUnknownForcesMandatory(t *testing.T) {
	got := effectiveOrder([]string{"machine", "ghostcolumn"}) // no name/status, one bogus key
	if slices.Contains(got, "ghostcolumn") {
		t.Errorf("unknown key not dropped: %v", got)
	}
	if !slices.Contains(got, "name") || !slices.Contains(got, "status") {
		t.Errorf("mandatory columns not forced in: %v", got)
	}
}

func TestColumnOrderTOMLRoundTrip(t *testing.T) {
	p := defaultPrefs()
	p.columnOrder = []string{"name", "status", "machine"}
	var f prefsFile
	if err := toml.Unmarshal([]byte(renderPrefsTOML(p)), &f); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.ColumnOrder, p.columnOrder) {
		t.Errorf("column_order round-trip: got %v, want %v", f.ColumnOrder, p.columnOrder)
	}
}
