// Command residual measures what ADR-0015 asks the reader to accept: background
// work whose end vigie never observes, leaving its session reading `working` until
// the session itself ends.
//
//	just residual
//	just residual args="--json"
//
// **It exists because that figure has been wrong three times, and every correction
// arrived by accident.** It read *one in eight* while a second delivery of the
// notification went unparsed (#818); it went out of date within days as the corpus
// grew and #834 widened what counts as background work; and its latest form counts
// stops the session declared and vigie does not read (#842) as signals that never
// came. Each number came from a throwaway script that no longer exists, so no
// reader could re-run the claim and disagree with it. A figure nobody can re-derive
// is not evidence, whatever provenance line sits beside it (#843).
//
// **It reads the parser's own counts rather than the transcripts a second time.**
// A tool that reimplements the closing rules measures the tool, not vigie — which
// is failure mode one above, exactly. Everything here is aggregation and
// formatting; whatever the parser comes to treat as background work, or as closed,
// is what these numbers follow.
//
// It reads the local transcript corpus, so its output describes one machine on one
// day. That is why every figure it prints carries both.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/transcript"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, clock.Now); err != nil {
		fmt.Fprintf(os.Stderr, "residual: %v\n", err)
		os.Exit(1)
	}
}

// staleAfter is how long a session must have been silent before an open launch is
// read as certainly over rather than possibly still running.
//
// It is **not** a rule about sessions — ADR-0015 refuses exactly that, and nothing
// here feeds a status. It is a reporting split, so a reader is not left to eyeball
// which residuals are stale, which is how that distinction is drawn by hand today.
const staleAfter = 2 * time.Hour

type report struct {
	Corpus            string `json:"corpus"`
	MeasuredAt        string `json:"measured_at"`
	Transcripts       int    `json:"transcripts"`
	Launches          int    `json:"launches"`
	Open              int    `json:"open"`
	Sessions          int    `json:"sessions_launching_any"`
	SessionsOpen      int    `json:"sessions_left_open"`
	OpenSilentSession int    `json:"sessions_left_open_and_silent"`
}

func run(args []string, out *os.File, now func() time.Time) error {
	fs := flag.NewFlagSet("residual", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the report as JSON instead of prose")
	root := fs.String("root", "", "transcript root (default: ~/.claude/projects)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir := *root
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".claude", "projects")
	}
	// One level deep, exactly what the watcher scans: a residual measured over
	// transcripts vigie never opens would describe something nobody sees.
	paths, err := filepath.Glob(filepath.Join(dir, "*", "*.jsonl"))
	if err != nil {
		return err
	}
	sort.Strings(paths)

	r := report{Corpus: dir, MeasuredAt: now().UTC().Format("2006-01-02")}
	for _, p := range paths {
		info, err := transcript.Parse(p)
		if err != nil {
			continue // an unreadable transcript is not a residual
		}
		r.Transcripts++
		if info.BackgroundLaunches == 0 {
			continue
		}
		r.Sessions++
		r.Launches += info.BackgroundLaunches
		r.Open += info.BackgroundOpen
		if info.BackgroundOpen == 0 {
			continue
		}
		r.SessionsOpen++
		if silent(info.LastActivity, now()) {
			r.OpenSilentSession++
		}
	}
	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}
	return write(out, r)
}

// silent reports whether the transcript has been quiet long enough that an open
// launch cannot still be running.
func silent(lastActivity string, now time.Time) bool {
	t, err := time.Parse(time.RFC3339, lastActivity)
	if err != nil {
		return false // no instant to judge from: counted as "cannot say"
	}
	return now.Sub(t) > staleAfter
}

func write(out *os.File, r report) error {
	// Both denominators, the per-session one first. A launch is not what an operator
	// looks at; a row is, and stating only the per-launch rate is what made the
	// price read as negligible (#835).
	_, err := fmt.Fprintf(out, `corpus %s, measured %s

  transcripts scanned                 %4d
  sessions launching background work  %4d
  launches                            %4d

residual
  launches still open                 %4d   %s
  sessions left with one open         %4d   %s
      silent for over 2h              %4d   certainly over, so certainly stale
      the rest                        %4d   the transcript cannot say
`,
		r.Corpus, r.MeasuredAt,
		r.Transcripts, r.Sessions, r.Launches,
		r.Open, rate(r.Open, r.Launches),
		r.SessionsOpen, rate(r.SessionsOpen, r.Sessions),
		r.OpenSilentSession,
		r.SessionsOpen-r.OpenSilentSession)
	return err
}

// rate renders a share as "one in N", the form both documents state it in.
func rate(n, total int) string {
	if n == 0 || total == 0 {
		return "none"
	}
	return fmt.Sprintf("one in %.0f", float64(total)/float64(n))
}
