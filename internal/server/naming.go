package server

import (
	"path"
	"strconv"

	"github.com/haribo/claude-vigie/internal/store"
)

// What a session is called, and how its faults are spelled, decided once here —
// the third family of the ADR-0011 migration (#618).
//
// Each of these rules existed in Go for the TUI and again in JavaScript for the
// dashboard, transcribed by hand. The naming rule existed a third time, in the
// GNOME indicator, and that copy was not a transcription but a *different* rule:
// it fell back to the project directory where the other two fell back to the
// short id, so one untitled session had two names depending on which window the
// operator was looking at. Nothing reported it, because nothing could.
//
// The boundary is the value, not its presentation: truncation, padding, column
// width and color stay in the client, because a fixed-width terminal cell and a
// browser table cell do not cut alike.

// shortIDLen is how much of a session id stands in for a missing title. Eight
// hexadecimal characters separate the sessions of a fleet without filling the
// column, and the full id stays one keystroke away in the detail panel.
const shortIDLen = 8

// nameView is what the session is called on screen: its title, else the short id.
func nameView(title, id string) string {
	if title != "" {
		return title
	}
	return shortID(id)
}

// shortID is the first shortIDLen characters of a session id. It counts runes,
// not bytes: an id is server-supplied and only checked for being non-empty, so a
// byte slice could cut a multi-byte character in half and emit an invalid rune.
func shortID(id string) string {
	r := []rune(id)
	if len(r) > shortIDLen {
		return string(r[:shortIDLen])
	}
	return id
}

// projectView is the final segment of a project directory, or a dash when the
// session carries none. `path` rather than `filepath`: the directory is reported
// by a watcher that may run on another operating system than the daemon, so the
// separator is the reported one, not the daemon's.
func projectView(dir string) string {
	if dir == "" {
		return "-"
	}
	return path.Base(dir)
}

// modeLabels is the #303 permission-mode taxonomy, raw value to short label.
// Vigilance rises manual → accept → plan → auto → bypass.
var modeLabels = map[string]string{
	"":                  "-",
	"default":           "manual",
	"acceptEdits":       "accept",
	"plan":              "plan",
	"auto":              "auto",
	"bypassPermissions": "bypass",
}

// modeDetails is the same taxonomy spelled out, for a detail panel that has the
// room a table cell does not.
//
//nolint:gosec // G101 false positive: operator-facing labels, not credentials — gosec matches on `permission`.
var modeDetails = map[string]string{
	"":                  "-",
	"default":           "manual — asks for permission",
	"acceptEdits":       "accept — auto-accepts edits",
	"plan":              "plan — awaiting plan approval",
	"auto":              "auto — runs unattended",
	"bypassPermissions": "bypass — no permission checks",
}

// modeLabelView and modeDetailView label a permission mode, carrying an
// unrecognized non-empty value through as it came. Never relabelled "manual": a
// mode this build has never heard of must not read as the safe default (#304),
// and the daemon is as capable of being older than Claude Code as a client is.
func modeLabelView(raw string) string  { return labelOr(modeLabels, raw) }
func modeDetailView(raw string) string { return labelOr(modeDetails, raw) }

func labelOr(table map[string]string, raw string) string {
	if l, ok := table[raw]; ok {
		return l
	}
	return raw
}

// apiErrorLabel names the Claude API error codes worth spelling out, and returns
// the bare code for the rest — an unnamed code is still the one thing that
// separates an outage from throttling, so it is never swallowed.
func apiErrorLabel(code int) string {
	switch code {
	case 429:
		return "429 Rate limited"
	case 500:
		return "500 Internal server error"
	case 529:
		return "529 Overloaded"
	default:
		return strconv.Itoa(code)
	}
}

// detailTextView is what the DETAIL cell shows, in precedence order.
//
// A raised call takes the cell: it is the reason the row is animated, and the
// operator needs it more than the tool that ran last (#389). An API error comes
// next — once the API answers 529 the last tool is of no interest (#584).
//
// `effective` rather than s.Status: a session whose reports went stale is shown
// under a status the store does not hold, and a derived field read from the
// stored one would describe a session the client is never shown (#617).
func detailTextView(s store.Session, effective string) string {
	if s.CallAt != "" {
		if s.CallMessage != "" {
			return s.CallMessage
		}
		return "called you"
	}
	if effective == "error" && s.APIErrorStatus != 0 {
		return apiErrorLabel(s.APIErrorStatus)
	}
	if s.Detail == "" {
		return "-"
	}
	return s.Detail
}
