package main

import (
	"regexp"
	"strconv"
)

// This file holds everything that decides what leaves the machine. It is
// separate from the loop because it is the part that has to be read carefully and
// tested exhaustively: [ADR-0016](../../docs/adr/0016-capture-claude-code-artefacts.md)
// turns "scrub before committing" into "never write it", and this is where that
// is either true or false.

// askShape matches the *opening* of the registry's `waitingFor`: the tool being
// asked about, and whether an argument follows. Deliberately anchored at the
// start and deliberately not trying to parse the rest — the rest is the part that
// carries a command line, a URL or a path.
var askShape = regexp.MustCompile(`^Allow ([A-Za-z][A-Za-z0-9_.-]*)(\()?`)

// redactAsk reduces `waitingFor` to its shape, and reports whether it recognized
// one. The argument is never returned in any form.
//
// **Unrecognized input fails closed.** The observed corpus for this field is two
// strings in a hand-written test fixture, which is precisely the kind of belief
// ADR-0016 exists to stop trusting: the real format may differ, and it may differ
// per tool. So anything that does not match returns no shape at all, and only the
// length survives — enough to learn that the format is not what we assumed,
// without learning what it said.
func redactAsk(s string) (shape string, known bool, runes int) {
	runes = len([]rune(s))
	if s == "" {
		return "", true, 0
	}
	m := askShape.FindStringSubmatch(s)
	if m == nil {
		return "", false, runes
	}
	if m[2] == "(" {
		return "Allow " + m[1] + "(…)?", true, runes
	}
	return "Allow " + m[1] + "?", true, runes
}

// aliaser hands out stable synthetic names within one recording, so a capture
// keeps *which records are the same thing across samples* — the only property a
// fixture needs from an identifier — while carrying none of the real one.
//
// Stability is per recording, not global: two recordings may give the same alias
// to different sessions, and nothing downstream may join them on it.
type aliaser struct {
	prefix string
	seen   map[string]string
}

func newAliaser(prefix string) *aliaser {
	return &aliaser{prefix: prefix, seen: map[string]string{}}
}

// of returns the alias for a real identifier, minting one on first sight. An empty
// input has no alias: absence is a fact a fixture needs to keep.
func (a *aliaser) of(id string) string {
	if id == "" {
		return ""
	}
	if got, ok := a.seen[id]; ok {
		return got
	}
	name := a.prefix + strconv.Itoa(len(a.seen)+1)
	a.seen[id] = name
	return name
}
