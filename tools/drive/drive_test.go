//go:build linux

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeRecord(t *testing.T, home, file, sessionID, status string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "sessions")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	body := `{"sessionId":"` + sessionID + `","status":"` + status + `"}`
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The whole instrument rests on clearing the markers a Claude Code inherits from
// the Claude Code that launched it: with CLAUDE_CODE_CHILD_SESSION in place the
// session writes no transcript and no registry record, so vigie cannot see it and
// the run measures nothing. The first version of this harness failed exactly there
// and looked like a bug in the harness (#853).
func TestTheInheritedChildMarkersAreCleared(t *testing.T) {
	t.Setenv("CLAUDE_CODE_CHILD_SESSION", "1")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("KEEP_ME", "yes")

	env := childEnv()

	for _, unwanted := range inherited {
		for _, kv := range env {
			if strings.HasPrefix(kv, unwanted+"=") {
				t.Errorf("%s survived into the session's environment; it turns the registry record off", unwanted)
			}
		}
	}
	if !contains(env, "KEEP_ME=yes") {
		t.Error("an unrelated variable was dropped; only the inherited markers may be removed")
	}
	if !contains(env, "TERM=xterm-256color") {
		t.Error("no TERM: Claude Code never reaches a typeable prompt without one")
	}
}

// A variable whose name merely starts with a marker's name is a different
// variable. Dropping it would silently change the session's environment.
func TestOnlyTheExactMarkersAreDropped(t *testing.T) {
	t.Setenv("CLAUDECODE_EXTRA", "keep")

	if !contains(childEnv(), "CLAUDECODE_EXTRA=keep") {
		t.Error("CLAUDECODE_EXTRA was dropped as if it were CLAUDECODE")
	}
}

// The harness cannot know the id of the session it just launched: Claude Code
// names it. It is read from the record that appeared, which is why the set of
// records is taken before the launch.
func TestTheSessionIDIsTheRecordThatAppeared(t *testing.T) {
	home := t.TempDir()
	writeRecord(t, home, "old.json", "s-old", "idle")
	before := registryFiles(home)
	writeRecord(t, home, "new.json", "s-new", "busy")

	if got := newSessionID(home, before); got != "s-new" {
		t.Errorf("session id = %q, want s-new", got)
	}
}

func TestRegistryStatusReadsClaudeCodesOwnWord(t *testing.T) {
	home := t.TempDir()
	writeRecord(t, home, "a.json", "s-1", "shell")
	writeRecord(t, home, "b.json", "s-2", "busy")

	if got := registryStatus(home, "s-2"); got != "busy" {
		t.Errorf("registry status = %q, want busy", got)
	}
	if got := registryStatus(home, "s-absent"); got != "" {
		t.Errorf("registry status = %q for a session with no record, want empty", got)
	}
}

// The verdict is the line the reader acts on, so it must fire on the shape #851
// was: Claude Code holding work while the board offers the session as free.
func TestTheVerdictNamesADisagreement(t *testing.T) {
	obs := []observation{
		{Elapsed: 0, Registry: "idle", Status: "idle"},
		{Elapsed: 10, Registry: "shell", Status: "idle", Detail: "shell"},
		{Elapsed: 20, Registry: "shell", Status: "idle", Detail: "shell"},
	}

	got := verdict(obs)
	if !strings.Contains(got, "DISAGREEMENT from t=10s") || !strings.Contains(got, `"shell"`) {
		t.Errorf("verdict = %q, want it to name the disagreement and the word Claude Code used", got)
	}
}

// The board is written after the watcher's next scan, so it is behind Claude Code
// by design. Reported raw, that lag fires on the control scenario at t=0 s — which
// is how this was found, on the tool's own first real run (#853).
func TestOneSampleOfLagIsNotADisagreement(t *testing.T) {
	obs := []observation{
		{Elapsed: 0, Registry: "busy", Status: "idle"}, // the board has not caught up yet
		{Elapsed: 10, Registry: "idle", Status: "idle"},
		{Elapsed: 20, Registry: "idle", Status: "idle"},
	}

	if got := verdict(obs); strings.Contains(got, "DISAGREEMENT") {
		t.Errorf("verdict = %q; one sample of propagation lag is not a disagreement", got)
	}
}

// A disagreement in the last sample is not dismissed as lag, but there is no
// second sample to confirm it either. Saying nothing is the honest answer: the run
// was too short, and inventing a verdict from one reading is the habit this whole
// tool exists against.
func TestADisagreementInTheLastSampleIsNotCalled(t *testing.T) {
	obs := []observation{
		{Elapsed: 0, Registry: "idle", Status: "idle"},
		{Elapsed: 10, Registry: "shell", Status: "idle"},
	}

	if got := verdict(obs); strings.Contains(got, "DISAGREEMENT") {
		t.Errorf("verdict = %q; a single trailing sample cannot settle it", got)
	}
}

func TestTheVerdictIsSilentWhenTheBoardAgrees(t *testing.T) {
	obs := []observation{
		{Elapsed: 10, Registry: "shell", Status: "working", Detail: "shell"},
		{Elapsed: 20, Registry: "idle", Status: "idle"},
	}

	if got := verdict(obs); strings.Contains(got, "DISAGREEMENT") {
		t.Errorf("verdict = %q, want no disagreement reported", got)
	}
}

// A table quoted in an issue or an ADR is only true of the version it was taken
// on, and a figure with no provenance is how ADR-0015's residual survived being
// wrong three times (#843).
func TestTheTableCarriesItsProvenance(t *testing.T) {
	var buf bytes.Buffer
	at := time.Date(2026, 9, 18, 15, 0, 0, 0, time.UTC)

	if err := render(&buf, scenarios[0], "0123456789", nil, false, at); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "2026-09-18") {
		t.Error("the table carries no date")
	}
	if !strings.Contains(out, "01234567") {
		t.Error("the table does not say which session it describes")
	}
}

func TestAnUnknownScenarioIsRefused(t *testing.T) {
	var buf bytes.Buffer

	err := run([]string{"-scenario", "nope"}, &buf, time.Now)

	if err == nil || !strings.Contains(err.Error(), "unknown scenario") {
		t.Errorf("err = %v, want it to refuse an unknown scenario before spending a session", err)
	}
}

// Running it with no scenario prints what it can do rather than starting one: a
// tool that spends tokens must not do so by default.
func TestNoScenarioPrintsTheChoicesAndStartsNothing(t *testing.T) {
	var buf bytes.Buffer

	err := run(nil, &buf, time.Now)

	if err == nil {
		t.Fatal("running with no scenario succeeded; it must refuse")
	}
	for _, s := range scenarios {
		if !strings.Contains(buf.String(), s.name) {
			t.Errorf("usage does not list the %q scenario", s.name)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
