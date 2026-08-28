package server

import (
	"reflect"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
)

// #514. visibleSignature decides whether a report changed anything a client
// displays, and therefore whether an SSE event is published. It hashed nineteen
// fields and omitted four the dashboard renders: Effort, ContextTokens,
// ContextKnown and PermissionMode.
//
// The web dashboard stops polling once the stream is live, so a session that
// switched to plan mode, changed reasoning effort or filled its context window
// never redrew. The TUI hides the defect by refetching every 5 s regardless.
//
// A dashboard that has stopped updating one column looks exactly like one where
// nothing changed — the failure #449 and #456 exist to prevent, arriving through
// the event layer instead of the refresh layer.
//
// The list is what drifted, and it drifted silently because nothing tied it to
// the fields it is meant to cover. So the test below is over `api.SessionView`,
// the contract the clients render: a field is either in the signature or in an
// explicit exclusion with a reason. Adding a field to the view now forces that
// decision instead of letting it be forgotten.

// signatureExcluded are the view fields deliberately outside the signature, with
// why. There are two grounds, and only two:
//
//   - the field changes on *every* report, so including it would publish an event
//     per report and defeat the point of having a signature at all — the clients
//     would be back to polling, over SSE;
//   - the field is *derived* from fields the signature already covers, so it
//     cannot change unless one of them does, and covering it again adds length
//     without adding a single event.
//
// Anything else is a field whose change would never reach an open dashboard.
var signatureExcluded = map[string]string{
	"LastSeenAt": "moves on every report; an event per report is what the signature exists to avoid",
	"Samples":    "a rolling window recomputed per read, not session state",
	"StartedAt":  "immutable after creation; a change is impossible",
	"EndedAt":    "always accompanied by a Status change, which is covered",
	// Derived by the daemon since ADR-0011 (#616, #617), from inputs already covered.
	"ContextWindow": "derived from Model, which is covered — it cannot change without it",
	"ContextPct":    "derived from Model and the context reading, all covered — it cannot change without one of them",
	// These two derive from the *effective* status, not the stored one: it turns
	// stale on its own when reports stop, is evaluated at read time, and is
	// already outside the signature by that design — clients learn of it by
	// asking again, not by an event. Covering the derivations would not change
	// that, only lengthen the string.
	"Attention": "derived from the effective status, which is evaluated at read time and outside the signature by design",
	"Rank":      "derived from the effective status, which is evaluated at read time and outside the signature by design",
	// The naming and label family (#618). Each is a pure function of covered
	// fields, so it cannot change unless one of them does.
	"Name":       "derived from Title and ID, both covered",
	"Project":    "derived from ProjectDir, which is covered",
	"ModelShort": "derived from Model, which is covered",
	"ModeLabel":  "derived from PermissionMode, which is covered",
	"ModeDetail": "derived from PermissionMode, which is covered",
	// DetailText reads the *effective* status for its API-error arm, like Attention
	// and Rank above; its other inputs — CallAt, CallMessage, APIErrorStatus,
	// Detail — are all covered.
	"DetailText": "derived from the call, the API error code and Detail, all covered, plus the effective status which is outside by design",
}

// fieldsCovered lists what the signature reads, by view field name. Kept beside
// the implementation so the two are edited together.
var fieldsCovered = []string{
	"ID", "Status", "StatusChangedAt", "Detail", "Title", "User", "Machine", "Model",
	"GitBranch", "ProjectDir", "LastTool", "RemoteControl", "RemoteURL", "APIErrorStatus",
	"CallAt", "CallMessage", "Usage", "Effort", "ContextTokens", "PermissionMode",
}

func TestEveryRenderedFieldIsInTheSignature(t *testing.T) {
	covered := map[string]bool{}
	for _, f := range fieldsCovered {
		covered[f] = true
	}
	view := reflect.TypeOf(api.SessionView{})
	for i := 0; i < view.NumField(); i++ {
		name := view.Field(i).Name
		if covered[name] {
			continue
		}
		if _, ok := signatureExcluded[name]; ok {
			continue
		}
		t.Errorf("SessionView.%s is rendered by the clients but neither covered by visibleSignature nor excluded with a reason — a change to it would never reach an open dashboard", name)
	}
}

// The list above is a claim; this checks it against the implementation. Changing
// each field alone must change the signature, or the claim is decoration.
func TestChangingAnyCoveredFieldChangesTheSignature(t *testing.T) {
	base := store.Session{ID: "s", Machine: "m", Status: "idle"}

	for _, c := range []struct {
		field string
		apply func(store.Session) store.Session
	}{
		{"Effort", func(s store.Session) store.Session { s.Effort = "high"; return s }},
		{"ContextTokens", func(s store.Session) store.Session { s.ContextTokens, s.ContextKnown = 1234, true; return s }},
		{"ContextKnown", func(s store.Session) store.Session { s.ContextKnown = true; return s }},
		{"PermissionMode", func(s store.Session) store.Session { s.PermissionMode = "plan"; return s }},
		{"Status", func(s store.Session) store.Session { s.Status = "working"; return s }},
		{"Detail", func(s store.Session) store.Session { s.Detail = "Edit x.go"; return s }},
		{"CallAt", func(s store.Session) store.Session { s.CallAt = "2026-08-16T12:00:00Z"; return s }},
		{"RemoteURL", func(s store.Session) store.Session { s.RemoteURL = "https://x"; return s }},
	} {
		if visibleSignature(base) == visibleSignature(c.apply(base)) {
			t.Errorf("a change to %s leaves the signature identical — an open dashboard would never redraw", c.field)
		}
	}
}

// ContextTokens is a pointer: "unknown" and "known to be zero" are different
// states the dashboard renders differently (#367), so the signature must tell
// them apart rather than flattening both to 0.
func TestAKnownZeroContextIsNotTheSameAsAnUnknownOne(t *testing.T) {
	unknown := store.Session{ID: "s", Machine: "m", Status: "idle"}
	knownZero := unknown
	knownZero.ContextKnown = true // a just-cleared session: known, and zero

	if visibleSignature(unknown) == visibleSignature(knownZero) {
		t.Error("an unknown context and a known zero produce the same signature — a session that just cleared never redraws")
	}
}
