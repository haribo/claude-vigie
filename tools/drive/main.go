//go:build linux

// Command drive runs a real Claude Code session on a pseudo-terminal, types into
// it, and records what Claude Code's own registry says next to what vigie shows —
// second by second, in one table.
//
// It answers a question no test in this repository can. A test only ever sees the
// words we wrote into a fixture ([ADR-0016](../../docs/adr/0016-capture-claude-code-artefacts.md)),
// so when the belief behind the fixture is wrong the suite is green and the board
// is wrong. That is not hypothetical: `shell` was read as "the operator is at a
// bash prompt" for six weeks and four fixes (#661, #748, #834, #842) until one run
// of this showed that the operator at a `!` prompt reports `busy`, and `shell`
// means a shell is running *for* Claude (#851, ADR-0017).
//
// `tools/capture` and `test/replay` answer the other half — replaying a shape once
// observed, so it cannot regress. This one is for finding out what the shape is.
//
//	just drive monitor
//	just drive bang "-seconds 200"
//	just drive foreground -json
//
// It is a development instrument, not a gate: it needs credentials, spends tokens,
// and is not deterministic. Measure with it, then freeze what it showed into
// `test/replay` (#817). It ships in no binary — hence `tools/`
// ([ADR-0003](../../docs/adr/0003-split-client-and-daemon-binaries.md)).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/config"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, clock.Now); err != nil {
		fmt.Fprintf(os.Stderr, "drive: %v\n", err)
		os.Exit(1)
	}
}

// scenario is what to type into the session and what it is meant to provoke.
//
// The keys are sent one entry at a time with a pause between them, because a
// terminal UI is not a pipe: text pasted and submitted in the same instant is
// submitted before the prompt has accepted it, and the run measures an idle
// session doing nothing.
type scenario struct {
	name  string
	about string
	keys  []string
}

// scenarios are the cases that decide a status rule. Each names the state it is
// meant to produce, so a run that produces something else is itself the finding —
// which is exactly what happened with `bang`.
var scenarios = []scenario{
	{
		name:  "bang",
		about: "the operator drops to a shell prompt and runs a long command",
		keys:  []string{"!", "sleep 90; echo FINISHED"},
	},
	{
		name:  "monitor",
		about: "Claude starts a Monitor, whose shell outlives the call that made it",
		keys: []string{"Use the Monitor tool once with command 'sleep 90; echo FINISHED', " +
			"persistent false, timeout_ms 600000. Do not explain."},
	},
	{
		name:  "foreground",
		about: "Claude runs a long command at the front and waits for it",
		keys: []string{"Use the Bash tool once, in the foreground, with command " +
			"'until [ -f /tmp/drive-done ]; do :; done' and timeout 120000. Do not explain."},
	},
	{
		name:  "background",
		about: "Claude backgrounds a command and carries on",
		keys: []string{"Use the Bash tool once with run_in_background true and command " +
			"'sleep 90; echo FINISHED'. Do not explain."},
	},
	{
		name:  "idle",
		about: "a session that answers and then rests — the control case",
		keys:  []string{"Reply with the single word: ok"},
	},
}

func scenarioNamed(name string) (scenario, bool) {
	for _, s := range scenarios {
		if s.name == name {
			return s, true
		}
	}
	return scenario{}, false
}

func run(args []string, out io.Writer, now func() time.Time) error {
	fs := flag.NewFlagSet("drive", flag.ContinueOnError)
	fs.SetOutput(out)
	name := fs.String("scenario", "", "which scenario to drive: "+strings.Join(names(), ", "))
	seconds := fs.Int("seconds", 150, "how long to sample after the keys are sent")
	every := fs.Int("every", 10, "seconds between samples")
	asJSON := fs.Bool("json", false, "print the run as JSON instead of a table")
	dir := fs.String("dir", "", "working directory for the session (default: a temporary one)")
	screens := fs.String("screens", os.TempDir(), "directory for the raw screen log of the session")
	fs.Usage = func() {
		fmt.Fprint(out, "usage: drive -scenario NAME [-seconds N] [-every N] [-json] [-dir PATH]\n\n"+
			"Runs a real Claude Code session on a pty and prints what Claude Code's registry\n"+
			"says next to what vigie shows. A development instrument: it spends tokens and is\n"+
			"not deterministic. See tools/drive/main.go.\n\nScenarios:\n")
		for _, s := range scenarios {
			fmt.Fprintf(out, "  %-12s %s\n", s.name, s.about)
		}
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		fs.Usage()
		return fmt.Errorf("no scenario given")
	}
	sc, ok := scenarioNamed(*name)
	if !ok {
		return fmt.Errorf("unknown scenario %q; known: %s", *name, strings.Join(names(), ", "))
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading the vigie config: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home: %w", err)
	}
	workDir := *dir
	if workDir == "" {
		workDir, err = os.MkdirTemp("", "drive-")
		if err != nil {
			return fmt.Errorf("making a working directory: %w", err)
		}
	}
	obs, sid, err := drive(sc, driveOpts{
		home: home, cfg: cfg, dir: workDir, screens: *screens,
		bin: "claude", binArgs: []string{"--permission-mode", "bypassPermissions"},
		seconds: *seconds, every: *every, now: now,
	})
	if err != nil {
		return err
	}
	return render(out, sc, sid, obs, *asJSON, now())
}

func names() []string {
	out := make([]string, 0, len(scenarios))
	for _, s := range scenarios {
		out = append(out, s.name)
	}
	sort.Strings(out)
	return out
}

type driveOpts struct {
	home string
	cfg  *config.Config
	dir  string
	// bin and binArgs are the program to put on the pty. They exist so the drive
	// loop can be run end to end against something trivial; the default is a real
	// Claude Code, and nothing else is ever measured.
	bin     string
	binArgs []string
	// waits are the pauses in driving a terminal. Zero means the defaults; a test
	// drives the loop with them collapsed, because what it checks is the sequence,
	// not how long Claude Code takes to paint a prompt.
	waits   waits
	screens string // where the raw screen log goes — never the working directory
	seconds int
	every   int
	now     func() time.Time
}

// bootWait is how long the session is given to reach a typeable prompt, and
// confirmWait how long the bypass-permissions screen is given to appear. Both are
// pauses in a script driving a terminal, not evidence about a session: nothing is
// derived from them, so ADR-0015 is not in play.
type waits struct {
	confirm time.Duration // before answering the bypass-permissions screen
	boot    time.Duration // before the prompt accepts typing
	key     time.Duration // between one keystroke group and the next
}

func (w waits) orDefaults() waits {
	if w.confirm == 0 {
		w.confirm = 6 * time.Second
	}
	if w.boot == 0 {
		w.boot = 12 * time.Second
	}
	if w.key == 0 {
		w.key = 1500 * time.Millisecond
	}
	return w
}

func drive(sc scenario, o driveOpts) ([]observation, string, error) {
	w := o.waits.orDefaults()
	before := registryFiles(o.home)
	// Out of the way by default: this runs from a checkout, and a tool that drops
	// a log beside the source it was run from is a tool that gets committed by
	// accident.
	screen := filepath.Join(o.screens, "drive-screen-"+sc.name+".log")
	s, err := startSession(o.bin, o.binArgs, o.dir, screen)
	if err != nil {
		return nil, "", err
	}
	defer s.close()

	// The bypass-permissions screen blocks startup until someone answers it: the
	// cursor starts on "No, exit", so this is Down then Enter.
	time.Sleep(w.confirm)
	if err := s.send("\x1b[B"); err != nil {
		return nil, "", err
	}
	time.Sleep(w.confirm / 12)
	if err := s.send("\r"); err != nil {
		return nil, "", err
	}
	time.Sleep(w.boot)

	sid := newSessionID(o.home, before)
	if sid == "" {
		return nil, "", fmt.Errorf("the session wrote no registry record — see startSession: " +
			"an inherited CLAUDE_CODE_CHILD_SESSION turns that record off, and `claude -p` never writes one")
	}

	for _, k := range sc.keys {
		if err := s.send(k); err != nil {
			return nil, sid, err
		}
		time.Sleep(w.key)
	}
	if err := s.send("\r"); err != nil {
		return nil, sid, err
	}

	var obs []observation
	start := o.now()
	for {
		elapsed := int(o.now().Sub(start).Seconds())
		if elapsed > o.seconds {
			break
		}
		status, detail := boardStatus(o.cfg, sid)
		obs = append(obs, observation{
			Elapsed:  elapsed,
			Registry: registryStatus(o.home, sid),
			Status:   status,
			Detail:   detail,
		})
		time.Sleep(time.Duration(o.every) * time.Second)
	}
	return obs, sid, nil
}

// render prints the run. The table is the output that matters: it is what an issue
// quotes, so it carries the Claude Code version and the date — a measurement with
// no provenance is how ADR-0015's figure survived being wrong three times (#843).
func render(out io.Writer, sc scenario, sessionID string, obs []observation, asJSON bool, at time.Time) error {
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"scenario":     sc.name,
			"about":        sc.about,
			"session":      sessionID,
			"claude_code":  claudeVersion(),
			"measured_at":  at.UTC().Format(time.RFC3339),
			"observations": obs,
		})
	}
	fmt.Fprintf(out, "scenario %s — %s\n", sc.name, sc.about)
	fmt.Fprintf(out, "session %s, Claude Code %s, %s\n\n", short(sessionID), claudeVersion(), at.UTC().Format("2006-01-02"))
	fmt.Fprintf(out, "%-6s  %-10s  %-10s  %s\n", "t", "registry", "vigie", "detail")
	for _, o := range obs {
		fmt.Fprintf(out, "%-6s  %-10s  %-10s  %s\n",
			fmt.Sprintf("%ds", o.Elapsed), orDash(o.Registry), orDash(o.Status), orDash(o.Detail))
	}
	fmt.Fprint(out, "\n"+verdict(obs)+"\n")
	return nil
}

// verdict names the one thing a reader should not have to spot by eye: whether the
// two columns ever disagreed about the session being at work.
//
// **A single sample is not a disagreement.** vigie is a poller: the watcher scans
// on its own cycle and the board is written after that, so the instant a turn
// starts Claude Code knows and the board does not. Reported raw, that lag fires on
// every run — the `idle` control scenario included, at t=0 s — and an instrument
// that cries wolf on its quietest case is one nobody reads. So a disagreement has
// to survive two consecutive samples before it is called one, which propagation
// never does and a wrong rule always does.
func verdict(obs []observation) string {
	for i, o := range obs {
		if !working(o.Registry) || o.Status != "idle" {
			continue
		}
		next := i + 1
		if next >= len(obs) {
			break // the run ended here: not enough evidence to call it
		}
		if working(obs[next].Registry) && obs[next].Status == "idle" {
			return fmt.Sprintf("DISAGREEMENT from t=%ds: Claude Code says %q, the board says idle.",
				o.Elapsed, o.Registry)
		}
	}
	return "No disagreement: the board never called the session idle while Claude Code had work in hand."
}

func working(registry string) bool { return registry == "busy" || registry == "shell" }

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
