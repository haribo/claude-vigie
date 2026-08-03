package tui

import "strings"

// Column layout — the operator picks which columns show and in what order, saved
// across launches (#308). A column is referenced by its stable key (its lowered
// header); prefs.columnOrder is the ordered list of *visible* keys, empty meaning
// the built-in default. Choosing a view is allowed and persistable (ADR-0007).

// key is the stable identifier of a column, derived from its header.
func (c column) key() string { return strings.ToLower(c.header) }

// mandatoryColumns are always shown and cannot be hidden — the table is
// meaningless without a name and a status.
var mandatoryColumns = map[string]bool{"name": true, "status": true}

// columnByKey indexes the built-in columns by key.
func columnByKey() map[string]column {
	m := make(map[string]column, len(columns))
	for _, c := range columns {
		m[c.key()] = c
	}
	return m
}

// columnKeys returns every column's key in the built-in default order.
func columnKeys() []string {
	keys := make([]string, len(columns))
	for i, c := range columns {
		keys[i] = c.key()
	}
	return keys
}

// effectiveOrder is the ordered list of visible column keys: the saved order (with
// unknown keys dropped and the mandatory ones forced in), or the built-in default
// when nothing is saved.
func effectiveOrder(order []string) []string {
	if len(order) == 0 {
		return columnKeys()
	}
	known := columnByKey()
	seen := map[string]bool{}
	out := make([]string, 0, len(order))
	for _, k := range order {
		if _, ok := known[k]; ok && !seen[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	for _, k := range columnKeys() { // never drop a mandatory column
		if mandatoryColumns[k] && !seen[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	return out
}

// activeColumns is the visible columns in display order — the base for the table.
func activeColumns(order []string) []column {
	byKey := columnByKey()
	vis := effectiveOrder(order)
	out := make([]column, 0, len(vis))
	for _, k := range vis {
		out = append(out, byKey[k])
	}
	return out
}

// pickerColumns is every column in picker order: the visible ones first (display
// order), then the hidden ones (built-in order), so the operator can toggle a
// hidden column back on.
func pickerColumns(order []string) []column {
	byKey := columnByKey()
	vis := effectiveOrder(order)
	seen := map[string]bool{}
	out := make([]column, 0, len(columns))
	for _, k := range vis {
		out = append(out, byKey[k])
		seen[k] = true
	}
	for _, c := range columns {
		if !seen[c.key()] {
			out = append(out, c)
		}
	}
	return out
}

// columnVisible reports whether key is currently shown.
func columnVisible(order []string, key string) bool {
	for _, k := range effectiveOrder(order) {
		if k == key {
			return true
		}
	}
	return false
}

// toggleColumn shows/hides a column (mandatory ones can't be hidden), returning
// the new order — materialized from the default the first time it's edited.
func toggleColumn(order []string, key string) []string {
	if mandatoryColumns[key] {
		return order
	}
	cur := append([]string(nil), effectiveOrder(order)...)
	for i, k := range cur {
		if k == key { // visible → hide
			return append(cur[:i], cur[i+1:]...)
		}
	}
	return append(cur, key) // hidden → show (at the end)
}

// moveColumn shifts a visible column one slot (dir -1 up, +1 down); no-op for a
// hidden column or an out-of-range move.
func moveColumn(order []string, key string, dir int) []string {
	cur := append([]string(nil), effectiveOrder(order)...)
	idx := -1
	for i, k := range cur {
		if k == key {
			idx = i
			break
		}
	}
	j := idx + dir
	if idx < 0 || j < 0 || j >= len(cur) {
		return order
	}
	cur[idx], cur[j] = cur[j], cur[idx]
	return cur
}
