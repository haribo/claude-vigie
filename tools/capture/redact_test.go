package main

import (
	"strings"
	"testing"
	"time"
)

// #817. This repository is public, and #589/#631 are the evidence that a
// nettoyage performed by a person does not hold: real machine and account names
// were replaced, the task closed as done, and twelve more were found two weeks
// later. ADR-0016 answers that by never writing the bytes, and this file is where
// that claim is either true or a comment.
//
// The headline test is not "does it produce the right shape". It is "does
// anything survive that should not" — stated as a property over inputs carrying
// the things that would actually hurt, and counted.

// secrets are substrings that must never appear in a recording, whatever the
// input looked like. Each stands for a real class: a command line, an absolute
// path, a host, a credential, a third party's name.
var secrets = []string{
	"git push origin main",
	"/home/someone/.ssh/id_ed25519",
	"internal.example.test",
	"sk-live-01H9XYZ",
	"acme-corp",
}

func TestNothingSensitiveSurvivesTheAskDescription(t *testing.T) {
	// Inputs shaped like what the registry might hold, including the one form that
	// was actually observed — two lower-case words — and the forms a hand-written
	// fixture once claimed it had.
	inputs := []string{
		"permit tool",
		"Allow Bash(git push origin main)?",
		"Allow Read(/home/someone/.ssh/id_ed25519)?",
		"Allow Bash(curl -H 'Authorization: Bearer sk-live-01H9XYZ' https://internal.example.test)?",
		"Continue working on acme-corp with the key sk-live-01H9XYZ?",
		"", // absence is a fact worth keeping, and must not crash
	}
	leaks := 0
	for _, in := range inputs {
		form, n := describeAsk(in)
		for _, sec := range secrets {
			if strings.Contains(form, sec) {
				t.Errorf("describeAsk(%q) leaked %q → %q", in, sec, form)
				leaks++
			}
		}
		if n != len([]rune(in)) {
			t.Errorf("describeAsk(%q) reported length %d, want %d", in, n, len([]rune(in)))
		}
	}
	if leaks > 0 {
		t.Fatalf("%d leak(s); a recording built on this would publish them", leaks)
	}
}

// What replaced two tests, and why they are gone rather than updated.
//
// `TestRecognisedShapes` pinned `Allow Bash(git push)?` → `Allow Bash(…)?`, and
// `TestAnUnknownFormatYieldsNoShapeAtAll` pinned the fail-closed arm beside it.
// Both described a parser for a format that does not exist: the first live prompt
// the recorder caught reported two lower-case words, no bracket and no question
// mark (#817). A test asserting the behavior of invented structure is not a guard,
// it is the invention restated — so the structure went and the tests went with it.
//
// What remains true is that an ask leaves as a form and a length. That is asserted
// above, and the skeleton's own invariant covers the rest.
func TestAnAskLeavesAsAFormAndALength(t *testing.T) {
	form, n := describeAsk("permit tool")
	if form != "aaaaaa aaaa" {
		t.Errorf("form = %q, want the class skeleton", form)
	}
	if n != 11 {
		t.Errorf("length = %d, want 11", n)
	}
	if form, n := describeAsk(""); form != "" || n != 0 {
		t.Errorf("an absent ask described as %q/%d, want empty — absence is a fact to keep", form, n)
	}
}

// An alias keeps the only property a fixture needs from an identifier — which
// rows are the same thing — and carries none of the identifier.
func TestAliasesAreStableDistinctAndCarryNothing(t *testing.T) {
	a := newAliaser("s")
	real1, real2 := "0a371a56-dead-beef-0000-000000000001", "1b9c1195-dead-beef-0000-000000000002"
	first := a.of(real1)
	if again := a.of(real1); again != first {
		t.Errorf("alias changed across samples: %q then %q", first, again)
	}
	if other := a.of(real2); other == first {
		t.Errorf("two sessions share the alias %q", other)
	}
	for _, name := range []string{first, a.of(real2)} {
		if strings.Contains(real1, name) || strings.Contains(real2, name) {
			t.Errorf("alias %q is a fragment of the real id", name)
		}
	}
	if a.of("") != "" {
		t.Error("an absent id was given an alias; absence is a fact to keep")
	}
}

// The transcript contributes one interval and nothing else. This pins that the
// scan finds the *last* dated line, ignores undated ones, and drops a leading
// fragment when the tail began mid-line.
func TestLastTimestampInReadsTheEndAndNothingElse(t *testing.T) {
	buf := []byte(`{"type":"assistant","timestamp":"2026-09-10T12:00:00Z"}
{"type":"attachment"}
{"type":"user","timestamp":"2026-09-10T12:05:00Z","message":{"content":"a secret"}}
`)
	got, ok := lastTimestampIn(buf, false)
	if !ok {
		t.Fatal("no timestamp found")
	}
	want := time.Date(2026, 9, 10, 12, 5, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("timestamp = %v, want %v", got, want)
	}

	// Began mid-line: the first fragment is not a line and must be discarded.
	partial := []byte(`p":"2026-01-01T00:00:00Z"}
{"timestamp":"2026-09-10T12:05:00Z"}
`)
	if got, ok := lastTimestampIn(partial, true); !ok || !got.Equal(want) {
		t.Errorf("mid-line tail = %v (ok=%v), want %v", got, ok, want)
	}
}

// #817. `redactAsk` fails closed, so the first prompt this tool recorded said only
// "the format is not what we assumed" and a length. That is the right amount to
// write and not enough to widen the pattern with — hence the skeleton, which names
// a string's form and none of its content.
//
// The property, and the first version of this test got it wrong in a way worth
// keeping a note of: asserting that "no letter of the input appears in the
// skeleton" fails on its own marker characters, since a skeleton is *made* of `a`,
// `A` and `9`. The invariant is not about which characters appear. It is that the
// skeleton depends only on each character's **class** and never on its identity —
// so rewriting the input with different letters, digits and scripts must produce
// exactly the same skeleton. If a single character leaked, the two would differ.
func TestASkeletonCarriesTheClassAndNothingElse(t *testing.T) {
	// classPreserving replaces every character with a *different* one of the same
	// class, leaving structural characters alone.
	classPreserving := func(s string) string {
		out := make([]rune, 0, len(s))
		for _, r := range s {
			switch {
			case r >= 'a' && r <= 'z':
				out = append(out, 'z'+'a'-r) // some other lower-case letter
			case r >= 'A' && r <= 'Z':
				out = append(out, 'Z'+'A'-r)
			case r >= '0' && r <= '9':
				out = append(out, '9'-r+'0')
			case r > 127:
				out = append(out, '\u4f60') // some other non-ASCII letter
			default:
				out = append(out, r)
			}
		}
		return string(out)
	}

	for _, in := range []string{
		"Allow Bash(git push origin main)?",
		"Allow Read(/home/someone/.ssh/id_ed25519)?",
		"Continuer avec le dépôt café-brûlé ?",
		"Разрешить Bash?",
		"sk-live-01H9XYZ",
		"\x00\x07 control",
		"🔥 emoji",
		"",
	} {
		a, b := skeleton(in), skeleton(classPreserving(in))
		if a != b {
			t.Errorf("skeleton leaks a character's identity:\n  %q -> %q\n  rewritten -> %q", in, a, b)
		}
	}
}

// And every class must actually be collapsed — a skeleton made of pass-through
// characters would satisfy the invariant above by doing nothing.
func TestEveryClassIsCollapsed(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"aZ9", "aA9"},
		{"é", "."},             // a letter in another script: fail closed
		{"\u4f60", "."},        // and another
		{"\x00", "."},          // a control character
		{"🔥", "."},             // an emoji
		{"()/-_ ?", "()/-_ ?"}, // structural characters survive, by allow list
	} {
		if got := skeleton(c.in); got != c.want {
			t.Errorf("skeleton(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// And it must still be *useful*: the form has to be legible, or it buys nothing
// over the length we already had.
func TestASkeletonShowsTheFormItWasBuiltFor(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Allow Bash(git push)?", "Aaaaa Aaaa(aaa aaaa)?"},
		{"Allow Bash?", "Aaaaa Aaaa?"},
		{"Run command 42 in /tmp", "Aaa aaaaaaa 99 aa /aaa"},
		{"", ""},
	} {
		if got := skeleton(c.in); got != c.want {
			t.Errorf("skeleton(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// An absurd field cannot dump its structure either.
func TestASkeletonIsBounded(t *testing.T) {
	if n := len([]rune(skeleton(strings.Repeat("ab(cd) ", 400)))); n > skeletonCap+1 {
		t.Errorf("skeleton is %d runes, want at most %d", n, skeletonCap+1)
	}
}
