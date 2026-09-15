package main

import (
	"strconv"
	"strings"
)

// This file holds everything that decides what leaves the machine. It is
// separate from the loop because it is the part that has to be read carefully and
// tested exhaustively: [ADR-0016](../../docs/adr/0016-capture-claude-code-artefacts.md)
// turns "scrub before committing" into "never write it", and this is where that
// is either true or false.

// describeAsk reports what may be written about the registry's `waitingFor`: the
// form of the question, and how long it was. Never a character of it.
//
// **There used to be a pattern here, and observation retired it.** It matched
// `Allow <Tool>(…)?` and returned that shape with the argument stripped, on the
// strength of `internal/watch/registry_test.go`, which had asserted
// `"waitingFor":"Allow Bash?"` since it was written. The first live permission
// prompt this recorder caught reported a form that matches none of it — two
// lower-case words, no bracket, no question mark (#817). The pattern could never
// have fired, and the argument-stripping it performed was structure invented to
// fit an invented fixture.
//
// A format that is only words has nothing to strip, so the skeleton is not a
// fallback here — it is the answer, and the only one that does not require reading
// the question.
func describeAsk(s string) (form string, runes int) {
	if s == "" {
		return "", 0
	}
	return skeleton(s), len([]rune(s))
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

// skeletonCap bounds the skeleton, so a field that stopped being a short question
// cannot dump its structure either.
const skeletonCap = 60

// skeleton describes a string's *form* without any of its content: an upper-case
// letter becomes `A`, a lower-case one `a`, a digit `9`, and everything else —
// spaces, brackets, punctuation — is kept as itself.
//
// It exists for one job. `redactAsk` fails closed, so the first real permission
// prompt this tool recorded reported only "the format is not what we assumed" and
// a length of 12 (#817). That is the right amount to *write*, and not enough to
// widen the pattern with: knowing the format would mean reading the string, and
// the recorder is built never to reveal it. A skeleton is the way out that relaxes
// nothing — `Allow Bash(git push)?` becomes `Aaaaa Aaaa(aaa aaaa)?`, which names
// the prefix and the delimiters and no word.
//
// **What it does cost, stated rather than hidden.** A skeleton reveals a string's
// length, its case pattern and its punctuation. For a permission question that is
// nothing. For a field that one day carried a credential it would reveal the
// credential's shape — never a character of it, but a shape. That is why it is
// emitted only for input `redactAsk` did *not* recognize: a known shape needs no
// skeleton, so the exposure exists only where there is something left to learn.
func skeleton(s string) string {
	out := make([]rune, 0, skeletonCap)
	for _, r := range s {
		if len(out) == skeletonCap {
			out = append(out, '…')
			break
		}
		out = append(out, skeletonRune(r))
	}
	return string(out)
}

// structural is the set of characters that pass through a skeleton unchanged:
// ASCII punctuation and the space. It is an **allow list**, and that is the whole
// of the safety here — the first version of this function collapsed the three
// ASCII ranges and let "everything else" through as itself, which passes an
// accented letter, a Cyrillic word or a line of CJK straight into the recording.
// A deny list of character classes has the same shape as a deny list of secrets,
// and fails the same way (ADR-0016).
const structural = " !\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

// skeletonRune reduces one character to its class, failing closed: anything not
// in `structural` and not an ASCII letter or digit becomes `.`, so a rune this
// function has never considered cannot pass through by default.
func skeletonRune(r rune) rune {
	switch {
	case r >= 'a' && r <= 'z':
		return 'a'
	case r >= 'A' && r <= 'Z':
		return 'A'
	case r >= '0' && r <= '9':
		return '9'
	case strings.ContainsRune(structural, r):
		return r
	default:
		return '.' // a letter in any other script, a control character, an emoji
	}
}
