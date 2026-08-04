package tui

import "strings"

// Column layout — the operator picks which columns show and in what order, saved
// across launches (#308). A column is referenced by its stable key (its lowered
// header). prefs.columnOrder is the display order of ALL columns (empty = the
// built-in default); prefs.columnHidden is the set of hidden keys. Hiding a column
// keeps its position — only its visibility changes (#315). Choosing a view is
// allowed and persistable (ADR-0007).

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

// fullOrder is every column's key in display order: the saved order (unknown keys
// dropped), with any column missing from it appended in the built-in order — so a
// layout survives added/removed columns. Empty order → the built-in default.
func fullOrder(order []string) []string {
	known := columnByKey()
	seen := map[string]bool{}
	out := make([]string, 0, len(columns))
	for _, k := range order {
		if _, ok := known[k]; ok && !seen[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	for _, k := range columnKeys() {
		if !seen[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	return out
}

// columnHidden reports whether key is hidden (mandatory columns never are).
func columnHidden(hidden []string, key string) bool {
	if mandatoryColumns[key] {
		return false
	}
	for _, k := range hidden {
		if k == key {
			return true
		}
	}
	return false
}

// activeColumns is the visible columns in display order — the base for the table:
// the full order minus the hidden ones.
func activeColumns(order, hidden []string) []column {
	byKey := columnByKey()
	out := make([]column, 0, len(columns))
	for _, k := range fullOrder(order) {
		if !columnHidden(hidden, k) {
			out = append(out, byKey[k])
		}
	}
	return out
}

// pickerColumns is every column in display order (stable — hiding never moves a
// column), for the Settings picker.
func pickerColumns(order []string) []column {
	byKey := columnByKey()
	out := make([]column, 0, len(columns))
	for _, k := range fullOrder(order) {
		out = append(out, byKey[k])
	}
	return out
}

// toggleColumn flips a column's visibility in the hidden set (mandatory ones can't
// be hidden). The display order is untouched, so the column keeps its place.
func toggleColumn(hidden []string, key string) []string {
	if mandatoryColumns[key] {
		return hidden
	}
	for i, k := range hidden {
		if k == key { // currently hidden → show
			return append(append([]string(nil), hidden[:i]...), hidden[i+1:]...)
		}
	}
	return append(append([]string(nil), hidden...), key) // show → hide
}

// moveColumn reorders a column one slot in the full display order (dir -1 up, +1
// down), materialized from the default the first time; no-op at the edges. Works
// for a hidden column too.
func moveColumn(order []string, key string, dir int) []string {
	cur := fullOrder(order)
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
