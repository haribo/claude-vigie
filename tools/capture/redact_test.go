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

func TestNothingSensitiveSurvivesRedaction(t *testing.T) {
	inputs := []string{
		"Allow Bash(git push origin main)?",
		"Allow Read(/home/someone/.ssh/id_ed25519)?",
		"Allow WebFetch(https://internal.example.test/api)?",
		"Allow Bash(curl -H 'Authorization: Bearer sk-live-01H9XYZ' https://internal.example.test)?",
		"Continue working on acme-corp with the key sk-live-01H9XYZ?",
		"Do you want to proceed with rm -rf /home/someone?",
		"", // absence is a fact worth keeping, and must not crash
	}
	leaks := 0
	for _, in := range inputs {
		shape, _, _ := redactAsk(in)
		for _, s := range secrets {
			if strings.Contains(shape, s) {
				t.Errorf("redactAsk(%q) leaked %q → %q", in, s, shape)
				leaks++
			}
		}
		// Stronger than the list: nothing inside the parentheses may survive at
		// all, named or not. A future secret is not in `secrets`.
		if i := strings.IndexByte(in, '('); i >= 0 && shape != "" {
			inner := in[i+1:]
			if j := strings.LastIndexByte(inner, ')'); j > 0 {
				if arg := inner[:j]; arg != "" && strings.Contains(shape, arg) {
					t.Errorf("redactAsk(%q) kept its argument → %q", in, shape)
					leaks++
				}
			}
		}
	}
	if leaks > 0 {
		t.Fatalf("%d leak(s); a recording built on this would publish them", leaks)
	}
}

// An unrecognized format must fail closed. The observed corpus for `waitingFor`
// is two strings in a hand-written fixture, so the real format may well differ —
// and the recorder has to teach us *that* without teaching us the content.
func TestAnUnknownFormatYieldsNoShapeAtAll(t *testing.T) {
	in := "Do you want to proceed with rm -rf /home/someone?"
	shape, known, n := redactAsk(in)
	if known {
		t.Errorf("claimed to recognize %q", in)
	}
	if shape != "" {
		t.Errorf("shape = %q, want empty — an unparsed string may not be echoed", shape)
	}
	if n != len([]rune(in)) {
		t.Errorf("length = %d, want %d — the length is what is left to learn from", n, len([]rune(in)))
	}
}

func TestRecognisedShapes(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Allow Bash?", "Allow Bash?"},
		{"Allow Bash(git push)?", "Allow Bash(…)?"},
		{"Allow WebFetch(https://x)?", "Allow WebFetch(…)?"},
		{"", ""},
	} {
		if got, known, _ := redactAsk(c.in); got != c.want || !known {
			t.Errorf("redactAsk(%q) = %q (known=%v), want %q", c.in, got, known, c.want)
		}
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
