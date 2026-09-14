package watch

import (
	"bytes"
	"strings"
	"testing"
)

// #817, and the lesson of #821. Claude Code's session registry is a private,
// undocumented format, and `mapRegistryStatus` reads a word it does not know as
// `idle`. Reading it that way is right — `idle` claims the least of any status.
// Saying nothing about it is not: a status renamed upstream would show every
// working session at rest, on every machine, with the suite green and nothing
// anywhere naming the cause.
//
// What these tests can and cannot do is the whole point. They cannot notice a
// rename: a test only ever sees words someone wrote down, and the rename is by
// definition a word nobody has. Only a watcher running against the real thing
// notices, so the detection is instrumentation. What is guarded here is that the
// instrument works, that it is not deafening, and that the vocabulary is one list
// rather than two — which is the defect #821 is about.

func TestAnUnknownRegistryStatusIsAnnouncedOnce(t *testing.T) {
	var out bytes.Buffer
	s := newScanner()
	s.errw = &out
	reg := map[string]sessionRecord{
		"a": {SessionID: "a", Status: "deliberating"}, // a word no build knows
		"b": {SessionID: "b", Status: "busy"},         // and one every build does
	}

	s.noteUnknownStatuses(reg)
	first := out.String()
	if !strings.Contains(first, `"deliberating"`) {
		t.Errorf("the unknown word was not named: %q", first)
	}
	if strings.Contains(first, "busy") {
		t.Errorf("a known word was announced: %q", first)
	}
	if !strings.Contains(first, "shown as idle") {
		t.Errorf("the notice does not say what the operator will see instead: %q", first)
	}

	// The scan runs every couple of seconds. Saying it again every time would bury
	// the notice in its own repetition.
	out.Reset()
	s.noteUnknownStatuses(reg)
	s.noteUnknownStatuses(reg)
	if again := out.String(); again != "" {
		t.Errorf("repeated on a later scan: %q", again)
	}
}

// The mapping and the "do we know this word" check must be one list. Two lists
// that have to agree, with one of them updated, is exactly how #816 was created.
// This fails if a word is ever mappable but reported unknown, or known but
// unmapped — whatever anyone adds later.
func TestTheVocabularyIsOneListNotTwo(t *testing.T) {
	for word := range registryStatuses {
		reg := map[string]sessionRecord{"s": {SessionID: "s", Status: word}}
		if got := unknownRegistryStatuses(reg); len(got) != 0 {
			t.Errorf("%q is mapped but reported unknown: %v", word, got)
		}
		if mapRegistryStatus(word) == "" {
			t.Errorf("%q is in the vocabulary but maps to nothing", word)
		}
	}
	// And the other direction: a word outside the list is reported, and still
	// reads as idle rather than as an empty status.
	reg := map[string]sessionRecord{"s": {SessionID: "s", Status: "brand-new"}}
	if got := unknownRegistryStatuses(reg); len(got) != 1 || got[0] != "brand-new" {
		t.Errorf("an unlisted word was not reported: %v", got)
	}
	if got := mapRegistryStatus("brand-new"); got != "idle" {
		t.Errorf("an unlisted word mapped to %q, want idle — it must claim the least", got)
	}
}

// The word is another program's output arriving in ours. A schema that stopped
// being an enum must not get to write a paragraph into the operator's log.
func TestAnAbsurdStatusIsCappedBeforeItIsPrinted(t *testing.T) {
	var out bytes.Buffer
	s := newScanner()
	s.errw = &out
	long := strings.Repeat("x", 5000)
	s.noteUnknownStatuses(map[string]sessionRecord{"s": {SessionID: "s", Status: long}})
	if n := out.Len(); n > 400 {
		t.Errorf("notice is %d bytes; an unbounded field reached the log", n)
	}
}
