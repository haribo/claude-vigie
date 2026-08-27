package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// sanitizeSessions cleans the fields it was written to clean, and #540 is what a
// hand-kept list costs: one field of thirteen was left out, and it was the one a
// row falls back to when a session has no title. The list has since grown by the
// six fields the daemon derives (#618), each built from the same transcript text.
//
// This walks the struct instead of the list. A string field added to SessionView
// and rendered without being sanitized fails here, which is the only way the next
// one does not repeat #540.
func TestEveryStringFieldOfTheSessionViewIsSanitized(t *testing.T) {
	// Fields whose content cannot be operator-hostile, each for a reason that is
	// checked rather than assumed.
	exempt := map[string]string{
		// Refused on ingest unless it is one of the known statuses
		// (rejectReport, internal/server/report.go).
		"Status": "closed vocabulary, validated by the daemon",
	}

	const hostile = "title\x1b]52;c;cGF5bG9hZA==\x07"

	dirty := api.SessionView{}
	rv := reflect.ValueOf(&dirty).Elem()
	planted := 0
	for i := 0; i < rv.NumField(); i++ {
		if rv.Type().Field(i).Type.Kind() == reflect.String {
			rv.Field(i).SetString(hostile)
			planted++
		}
	}
	if planted == 0 {
		t.Fatal("no string field found — the walk is broken, not the code")
	}

	clean := reflect.ValueOf(sanitizeSessions([]api.SessionView{dirty})[0])
	for i := 0; i < clean.NumField(); i++ {
		f := clean.Type().Field(i)
		if f.Type.Kind() != reflect.String {
			continue
		}
		if why, ok := exempt[f.Name]; ok {
			t.Logf("SessionView.%s exempt: %s", f.Name, why)
			continue
		}
		if strings.ContainsFunc(clean.Field(i).String(), isControl) {
			t.Errorf("SessionView.%s reaches the terminal with control characters intact — "+
				"add it to sanitizeSessions, or exempt it here with the reason", f.Name)
		}
	}
}
