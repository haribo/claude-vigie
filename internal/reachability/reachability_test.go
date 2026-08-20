package reachability

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "vigie-reachability-tests")
	if err != nil {
		panic("isolating the test home: " + err.Error())
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.Setenv("HOME", dir); err != nil {
		panic("isolating the test home: " + err.Error())
	}
	os.Exit(m.Run())
}

var now = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

func TestAnUnmarkedServerIsReachable(t *testing.T) {
	if Unreachable("http://never-marked:8080", now) {
		t.Error("a server with no mark must read reachable")
	}
}

func TestAMarkSuppressesUntilItGoesStale(t *testing.T) {
	const url = "http://host:8080"
	for _, c := range []struct {
		name string
		age  time.Duration
		want bool
	}{
		{"just written", 0, true},
		{"inside the window", StaleAfter - time.Second, true},
		{"exactly at the window", StaleAfter, true},
		{"past the window", StaleAfter + time.Second, false},
		{"slightly in the future — ordinary skew", -time.Second, true},
		{"far in the future — a clock step must not suppress forever", -2 * StaleAfter, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := Mark(url, now.Add(-c.age), errors.New("boom")); err != nil {
				t.Fatal(err)
			}
			if got := Unreachable(url, now); got != c.want {
				t.Errorf("Unreachable at age %s = %v, want %v", c.age, got, c.want)
			}
		})
	}
}

// The mark is per server, not per machine: `just dev-down` stopping the
// development daemon must not silence the production hooks (§ 4).
func TestOneServerMarkDoesNotSuppressAnother(t *testing.T) {
	const dev, prod = "http://localhost:8099", "http://vigie.example:8080"
	if err := Mark(dev, now, errors.New("dev is down")); err != nil {
		t.Fatal(err)
	}
	if !Unreachable(dev, now) {
		t.Error("the marked server must read unreachable")
	}
	if Unreachable(prod, now) {
		t.Error("marking the development daemon suppressed reports to production")
	}
}

// The callers trim the trailing slash when they build request URLs, so the two
// spellings must not become two marks — one would suppress and the other not.
func TestATrailingSlashIsTheSameServer(t *testing.T) {
	if err := Mark("http://host:8080/", now, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if !Unreachable("http://host:8080", now) {
		t.Error("http://host:8080/ and http://host:8080 must be one daemon")
	}
}

func TestClearForgetsTheMark(t *testing.T) {
	const url = "http://cleared:8080"
	if err := Mark(url, now, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if err := Clear(url); err != nil {
		t.Fatal(err)
	}
	if Unreachable(url, now) {
		t.Error("a cleared mark must not suppress")
	}
}

func TestClearingWhatWasNeverMarkedIsNotAnError(t *testing.T) {
	if err := Clear("http://never-marked-either:8080"); err != nil {
		t.Errorf("Clear on a missing mark: %v", err)
	}
}

// The body is for a human reading the state directory; nothing parses it, but an
// empty one would make the directory useless for the operator debugging an
// outage.
func TestTheMarkSaysWhichServerAndWhy(t *testing.T) {
	const url = "http://explained:8080"
	if err := Mark(url, now, errors.New("context deadline exceeded")); err != nil {
		t.Fatal(err)
	}
	p, err := pathFor(url)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(p) //nolint:gosec // the path is this package's own
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{url, "2026-08-18T12:00:00Z", "context deadline exceeded"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the mark body %q does not mention %q", body, want)
		}
	}
}
