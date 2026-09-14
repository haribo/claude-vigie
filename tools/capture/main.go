// Command capture records what Claude Code writes about its own sessions, so a
// test fixture can be observed rather than invented
// ([ADR-0016](../../docs/adr/0016-capture-claude-code-artefacts.md)).
//
// Run it and leave it running; it catches what happens rather than asking anyone
// to perform it:
//
//	just capture out=/tmp/capture.jsonl
//	just capture out=/tmp/capture.jsonl until-waiting=true
//
// It reads the files the watcher already reads and writes nowhere near them
// ([ADR-0005](../../docs/adr/0005-observe-only.md)).
//
// **What it does not write is the point.** Of the five registry fields the watcher
// reads, one is free text (`waitingFor`) and it leaves here as a shape, decided in
// redact.go; identifiers leave as per-recording aliases; and the transcript
// contributes one interval, never an instant and never a line. The output is
// meant to be readable by the person it was recorded on, which is the standard it
// has to meet before anything derived from it is committed.
//
// It is a development tool and ships in no binary — hence `tools/`, not a mode of
// the watcher: [ADR-0003](../../docs/adr/0003-split-client-and-daemon-binaries.md)
// keeps the client minimal, and capture has no business inside it.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/haribo/claude-vigie/internal/clock"
)

func main() {
	if err := run(os.Args[1:], clock.Now); err != nil {
		fmt.Fprintf(os.Stderr, "capture: %v\n", err)
		os.Exit(1)
	}
}

// options is everything the loop needs, so the loop itself reads no environment
// and no clock of its own.
type options struct {
	out          string
	interval     time.Duration
	untilWaiting bool
	lingerAfter  time.Duration
	sessionsDir  string
	projectsDir  string
}

func run(args []string, now func() time.Time) error {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	out := fs.String("out", "", "file to append the recording to (required; keep it outside the repository)")
	interval := fs.Duration("interval", 400*time.Millisecond, "how often to sample the registry")
	untilWaiting := fs.Bool("until-waiting", false, "stop once a `waiting` has been recorded, plus the linger window")
	linger := fs.Duration("linger", 90*time.Second, "how much longer to record after the first `waiting`")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("--out is required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return record(options{
		out: *out, interval: *interval, untilWaiting: *untilWaiting, lingerAfter: *linger,
		sessionsDir: filepath.Join(home, ".claude", "sessions"),
		projectsDir: filepath.Join(home, ".claude", "projects"),
	}, now)
}

// line is one row of the recording. Every field is either a synthetic alias, a
// closed vocabulary word, or an interval.
type line struct {
	AtMs    int64  `json:"at_ms"`             // offset from the start of this recording
	Event   string `json:"event,omitempty"`   // set on a recording event rather than a change
	GapMs   int64  `json:"gap_ms,omitempty"`  // wall-clock time unaccounted for by sampling
	Session string `json:"session,omitempty"` // per-recording alias
	Proc    string `json:"proc,omitempty"`    // per-recording alias for (pid, procStart)
	Status  string `json:"status,omitempty"`  // Claude Code's own word, a closed set
	Ask     string `json:"ask,omitempty"`     // `waitingFor`, reduced to its shape
	AskRaw  bool   `json:"ask_unknown,omitempty"`
	// AskSkel is the class skeleton of an ask whose shape was not recognized, and
	// only then: a known shape has nothing left to learn, so the exposure exists
	// exactly where it buys something (see skeleton in redact.go).
	AskSkel string `json:"ask_skeleton,omitempty"`
	AskLen  int    `json:"ask_len,omitempty"` // rune length of the original
	AgeMs   *int64 `json:"transcript_age_ms,omitempty"`
}

// recorder holds the state a recording accumulates: the aliases it has minted and
// the last value seen per registry file, so only changes are written.
type recorder struct {
	sessions *aliaser
	procs    *aliaser
	prev     map[string]entry
	started  time.Time
	out      *os.File
}

func record(o options, now func() time.Time) error {
	f, err := os.OpenFile(o.out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // the operator names this path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	start := now()
	r := &recorder{
		sessions: newAliaser("s"), procs: newAliaser("p"),
		prev: map[string]entry{}, started: start, out: f,
	}
	r.emit(line{Event: "start"}, start)

	var firstWaiting time.Time
	last := start
	for {
		t := now()
		// A jump far beyond the sampling interval means the machine slept. Recording
		// it is not bookkeeping: a timeline that hides its gaps claims a continuity
		// of observation it did not have, and the first version of this recorder lost
		// a whole night that way while reporting six hours of "watching".
		if d := t.Sub(last); d > o.interval*10 {
			r.emit(line{Event: "gap", GapMs: d.Milliseconds()}, t)
		}
		last = t

		if r.sweep(o, t) && firstWaiting.IsZero() {
			firstWaiting = t
		}
		if o.untilWaiting && !firstWaiting.IsZero() && t.Sub(firstWaiting) > o.lingerAfter {
			r.emit(line{Event: "captured a waiting, stopping"}, t)
			return nil
		}
		time.Sleep(o.interval)
	}
}

// sweep writes a row for every registry entry whose fields changed, and reports
// whether any of them said `waiting`.
func (r *recorder) sweep(o options, t time.Time) (sawWaiting bool) {
	cur := readRegistry(o.sessionsDir)
	for id, rec := range cur {
		if rec.Status == "waiting" {
			sawWaiting = true
		}
		if p, ok := r.prev[id]; ok && p == rec {
			continue
		}
		r.prev[id] = rec
		r.emit(r.rowFor(o, rec, t), t)
	}
	for id := range r.prev {
		if _, ok := cur[id]; !ok {
			delete(r.prev, id)
			r.emit(line{Event: "session left the registry", Session: r.sessions.of(id)}, t)
		}
	}
	return sawWaiting
}

// rowFor turns a registry record into the row that may be written: aliases for the
// identifiers, a shape for the question, an interval for the transcript.
func (r *recorder) rowFor(o options, rec entry, t time.Time) line {
	shape, known, n := redactAsk(rec.WaitingFor)
	l := line{
		Session: r.sessions.of(rec.SessionID),
		Status:  rec.Status,
		Ask:     shape,
		AskRaw:  !known,
		AskLen:  n,
	}
	if !known && rec.WaitingFor != "" {
		l.AskSkel = skeleton(rec.WaitingFor)
	}
	if rec.PID > 0 && rec.ProcStart > 0 {
		l.Proc = r.procs.of(fmt.Sprintf("%d/%d", rec.PID, rec.ProcStart))
	}
	if age, ok := transcriptAge(o.projectsDir, rec.SessionID, t); ok {
		ms := age.Milliseconds()
		l.AgeMs = &ms
	}
	return l
}

// emit stamps a row with its offset into the recording and appends it. A failed
// write stops nothing: losing a sample costs one row, and aborting the recording
// costs the scenario it was waiting for.
func (r *recorder) emit(l line, t time.Time) {
	l.AtMs = t.Sub(r.started).Milliseconds()
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	_, _ = r.out.Write(append(b, '\n'))
}
