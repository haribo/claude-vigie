//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/config"
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

// The pty plumbing itself, run against a trivial program. A terminal echoes what
// is typed at it, so `cat` proves the whole chain at once: the master/slave pair
// is wired, the session has a controlling terminal, keystrokes reach it, and the
// drain writes what came back. None of that depends on Claude Code being the
// program on the other end — and a harness whose own mechanics are first exercised
// while spending tokens is what this tool exists to stop.
func TestThePtyCarriesKeystrokesAndRecordsTheScreen(t *testing.T) {
	screen := filepath.Join(t.TempDir(), "screen.log")

	s, err := startSession("cat", nil, t.TempDir(), screen)
	if err != nil {
		t.Fatalf("starting a session on a pty: %v", err)
	}
	defer s.close()

	if err := s.send("hello\r"); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitFor(t, func() bool {
		b, err := os.ReadFile(screen) //nolint:gosec // a path this test made
		return err == nil && strings.Contains(string(b), "hello")
	})
}

// A program that is not there must fail at the launch, not leave a half-open pty
// and a run that measures nothing.
func TestAMissingProgramIsReportedAtTheLaunch(t *testing.T) {
	screen := filepath.Join(t.TempDir(), "screen.log")

	_, err := startSession("definitely-not-a-program", nil, t.TempDir(), screen)

	if err == nil {
		t.Fatal("starting a missing program succeeded")
	}
	if !strings.Contains(err.Error(), "definitely-not-a-program") {
		t.Errorf("err = %v, want it to name the program that could not start", err)
	}
}

// boardStatus asks the daemon what the operator sees, so it reads the same
// endpoint every other client does — and says so plainly when the session is not
// on the board, rather than passing an empty status off as `idle`.
func TestTheBoardIsReadThroughItsOwnEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sessions" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `[{"id":"s-1","name":"one","machine":"m","project_dir":"/x","project":"x","status":"working","detail_text":"shell","mode_label":"-","mode_detail":"-"}]`)
	}))
	defer srv.Close()
	cfg := &config.Config{ServerURL: srv.URL, Token: "t"}

	status, detail := boardStatus(cfg, "s-1")
	if status != "working" || detail != "shell" {
		t.Errorf("(%q, %q), want (working, shell)", status, detail)
	}

	if status, _ := boardStatus(cfg, "s-absent"); status != "absent" {
		t.Errorf("status = %q for a session the board does not carry, want absent", status)
	}
}

// An unreachable daemon is reported as such. Reading it as `idle` would invent the
// very disagreement this tool is here to detect.
func TestAnUnreachableBoardIsNotReadAsIdle(t *testing.T) {
	cfg := &config.Config{ServerURL: "http://127.0.0.1:1", Token: "t"}

	if status, _ := boardStatus(cfg, "s-1"); status != "unreachable" {
		t.Errorf("status = %q with no daemon, want unreachable", status)
	}
}

// The JSON form is what a script or an issue attachment consumes; it carries the
// same provenance the table does.
func TestTheJSONFormCarriesTheRunAndItsProvenance(t *testing.T) {
	var buf bytes.Buffer
	obs := []observation{{Elapsed: 10, Registry: "shell", Status: "idle", Detail: "shell"}}

	if err := render(&buf, scenarios[0], "s-1", obs, true, time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	var got struct {
		Scenario     string        `json:"scenario"`
		Session      string        `json:"session"`
		MeasuredAt   string        `json:"measured_at"`
		Observations []observation `json:"observations"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("the JSON form does not parse: %v", err)
	}
	if got.Scenario != scenarios[0].name || got.Session != "s-1" || got.MeasuredAt == "" {
		t.Errorf("run = %+v, want it to name the scenario, the session and when it was measured", got)
	}
	if len(got.Observations) != 1 || got.Observations[0].Registry != "shell" {
		t.Errorf("observations = %+v, want the row as measured", got.Observations)
	}
}

// Every scenario is reachable by the name it advertises: a scenario listed in the
// usage and not resolvable is a run someone cannot start.
func TestEveryListedScenarioResolves(t *testing.T) {
	for _, name := range names() {
		if _, ok := scenarioNamed(name); !ok {
			t.Errorf("scenario %q is listed but does not resolve", name)
		}
	}
	if _, ok := scenarioNamed("nope"); ok {
		t.Error("an unknown name resolved to a scenario")
	}
}

func TestADashStandsInForWhatWasNotObserved(t *testing.T) {
	if got := orDash(""); got != "-" {
		t.Errorf("orDash(\"\") = %q, want -", got)
	}
	if got := orDash("shell"); got != "shell" {
		t.Errorf("orDash(\"shell\") = %q, want shell", got)
	}
	if got := short("0123456789"); got != "01234567" {
		t.Errorf("short = %q, want the first eight characters", got)
	}
	if got := short("abc"); got != "abc" {
		t.Errorf("short = %q, want a short id left alone", got)
	}
}

// quickWaits collapses the terminal-driving pauses: a test checks the sequence,
// not how long Claude Code takes to paint a prompt.
var quickWaits = waits{confirm: 12 * time.Millisecond, boot: 30 * time.Millisecond, key: time.Millisecond}

// waitFor polls until cond holds, so a test does not depend on how fast a process
// gets scheduled.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition never held")
}

// The drive loop, run end to end against a program that is not Claude Code and so
// writes no registry record. It must stop there and say why, because that is the
// one failure a reader will actually hit: an inherited CLAUDE_CODE_CHILD_SESSION,
// or `claude -p`, both of which produce a session that runs and cannot be seen.
// Reported as "no data" it looks like a bug in the harness — which is how the
// first version of this was read (#853).
func TestASessionThatWritesNoRecordStopsAndSaysWhy(t *testing.T) {
	home := t.TempDir()

	_, _, err := drive(scenarios[0], driveOpts{
		home: home, cfg: &config.Config{}, dir: t.TempDir(), screens: t.TempDir(),
		bin: "cat", seconds: 0, every: 1, now: time.Now, waits: quickWaits,
	})

	if err == nil {
		t.Fatal("a session with no registry record was driven to the end")
	}
	if !strings.Contains(err.Error(), "CLAUDE_CODE_CHILD_SESSION") {
		t.Errorf("err = %v, want it to name what turns the record off", err)
	}
}

// With a record in place the loop samples to the end and returns the session it
// measured. `cat` stands in for the session: what is exercised here is the loop,
// not Claude Code.
func TestTheLoopSamplesUntilItsHorizon(t *testing.T) {
	home := t.TempDir()
	go func() {
		time.Sleep(10 * time.Millisecond)
		writeRecord(t, home, "new.json", "s-driven", "shell")
	}()

	obs, sid, err := drive(scenarios[4], driveOpts{
		home: home, cfg: &config.Config{ServerURL: "http://127.0.0.1:1"}, dir: t.TempDir(),
		screens: t.TempDir(), bin: "cat", seconds: 0, every: 1, now: time.Now, waits: quickWaits,
	})

	if err != nil {
		t.Fatalf("drive: %v", err)
	}
	if sid != "s-driven" {
		t.Errorf("session = %q, want the one that appeared in the registry", sid)
	}
	if len(obs) == 0 || obs[0].Registry != "shell" {
		t.Errorf("observations = %+v, want Claude Code's own word on the first row", obs)
	}
}
