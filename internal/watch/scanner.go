// Scanning the transcripts: the per-run cache, the process index and the
// session lineage the scan carries between passes.
package watch

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/transcript"
	"github.com/haribo/claude-vigie/internal/version"
)

// cacheEntry is a transcript's last parse, keyed by path and validated by the
// file's mod time and size.
type cacheEntry struct {
	modTime time.Time
	size    int64
	// parser carries the cumulative parse state, so a growing transcript is
	// advanced by only its new bytes instead of being re-parsed whole (#257).
	parser *transcript.Parser
}

// procID identifies a live Claude Code process — the {pid, /proc start-time} pair
// vigie already uses to tell a running process from a reused PID. Two session ids
// sharing a procID are the same lineage: a `/clear` switched the id in place on
// the same process (#367).
type procID struct {
	pid       int
	procStart uint64
}

// lineageEntry is the last known model and effort of the live session seen on a
// process, so a fresh session that replaces it (a `/clear`, which starts with no
// assistant line) inherits them until it writes its own first turn (#367).
type lineageEntry struct {
	model  string
	effort string
}

// scanner scans transcripts and caches each parse, so an unchanged (idle)
// transcript is not re-parsed every interval — important because a large
// transcript takes seconds to parse and the watcher scans frequently.
type scanner struct {
	cache map[string]cacheEntry
	// warnedStatus remembers which unknown registry status words have already been
	// announced, so each is said once for the life of the watcher rather than on
	// every scan. The scan runs every couple of seconds; a per-scan line would
	// bury the notice in its own repetition, which is why the drift and heartbeat
	// notices in watch.go are written as transitions too.
	warnedStatus map[string]bool
	// errw is where operator notices go. A seam rather than os.Stderr inline, so a
	// test can read what the operator would have been told without swapping
	// process-wide state (docs/code.md).
	errw io.Writer
	// lineage carries each live process's last known model/effort across scans,
	// keyed by process identity, so a `/clear`'d session inherits them instead of
	// showing "-" until its first turn. Pruned to live processes each scan (#367).
	lineage map[procID]lineageEntry
}

func newScanner() *scanner {
	return &scanner{
		cache: map[string]cacheEntry{}, lineage: map[procID]lineageEntry{},
		warnedStatus: map[string]bool{}, errw: os.Stderr,
	}
}

// Scan performs a single cache-less scan (used in tests and one-offs).
func Scan(root, machine string, maxAge time.Duration, now time.Time) ([]api.ReportRequest, error) {
	return newScanner().scan(root, machine, maxAge, now)
}

// scan reads every transcript under root modified within maxAge and returns a
// report (with a derived status) for each, reusing cached parses.
func (s *scanner) scan(root, machine string, maxAge time.Duration, now time.Time) ([]api.ReportRequest, error) {
	paths, err := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("globbing transcripts: %w", err)
	}

	osUser := systemUser()
	reg := readRegistry()         // Claude Code's authoritative status source (#254)
	s.noteUnknownStatuses(reg)    // say so before reading an unknown word as idle (#817)
	regByProc := indexByProc(reg) // process identity → its current session id (#367)
	var reports []api.ReportRequest
	fresh := make(map[string]cacheEntry, len(s.cache))
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		// The window bounds how much disk history is re-read on every scan; it must
		// not decide whether a session is alive. A session Claude Code still lists
		// is one whose process is running — the registry carries its pid — and
		// skipping it left the server with nothing to report, which it reads on a
		// watched machine as `ended`. A session open on the operator's screen was
		// announced as over (#660).
		//
		// The exception is bounded by the registry, so the extra parses are bounded
		// by live Claude processes rather than by the hundreds of transcripts a
		// working machine accumulates. The id is the file's own name, so this costs
		// no parse to decide.
		age := now.Sub(fi.ModTime())
		if age > maxAge {
			if _, listed := reg[strings.TrimSuffix(filepath.Base(p), ".jsonl")]; !listed {
				continue
			}
		}
		info, err := s.parse(p, fi, fresh)
		if err != nil {
			continue
		}

		// A transcript with no exchange in it is a metadata sidecar, not a session
		// — unless Claude Code still has it registered, which is what a session
		// you have started and not typed into looks like. That one is live and
		// must be shown; an abandoned sidecar, left behind by a renamed or moved
		// project, must not (#448).
		if !info.HasTurns {
			if _, live := reg[info.SessionID]; !live {
				continue
			}
		}

		// Prefer the last dated transcript line over the file mtime for "when did
		// this session last really do something". A live Claude appends
		// untimestamped metadata (last-prompt, bridge-session) roughly hourly,
		// bumping mtime without any activity; LastActivity ignores those lines, so
		// SEEN and the age-based status stay truthful. Fall back to mtime when no
		// dated line exists yet (a brand-new transcript).
		//
		// This comment used to add that the churn kept a live session inside the
		// scan window, "never expired while its process lives". That was the
		// argument for letting the window gate liveness, and it does not hold: the
		// metadata lines above are written when something *happens* — a prompt, a
		// mode change, a bridge sync — so a session nobody touches writes none.
		// The window no longer decides, the registry does (#660).
		lastActivity := fi.ModTime()
		if t, err := time.Parse(time.RFC3339, info.LastActivity); err == nil {
			lastActivity = t
		}
		activityAge := now.Sub(lastActivity)

		id := info.SessionID
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(p), ".jsonl")
		}
		usage := info.Usage
		model, effort := s.trackLineage(reg, id, info)
		status, activity, reportAt, declared := resolveStatus(reg, regByProc, id, info, activityAge, lastActivity, now)
		apiErr := 0
		if status == "error" {
			apiErr = info.LastAPIError // carry the HTTP code only while the error is shown
		}
		// The watcher parses the transcript every scan, so context is always a known
		// reading — a pointer distinguishes a known 0 (a just-cleared session, 0%)
		// from absent (#367).
		ctx := info.ContextTokens
		reports = append(reports, api.ReportRequest{
			Event:          "watch",
			SessionID:      id,
			User:           osUser,
			Machine:        machine,
			ProjectDir:     info.Cwd,
			GitBranch:      info.GitBranch,
			Model:          model,
			Effort:         effort,
			ContextTokens:  &ctx,
			PermissionMode: info.PermissionMode,
			Title:          info.Title,
			Status:         status,
			StatusDeclared: declared,
			Usage:          &usage,
			APIErrorStatus: apiErr,
			Detail:         activity,
			WatcherVersion: version.Version, // report this watcher's build (#356)
			WatcherCommit:  version.Commit,
			Timestamp:      reportAt.UTC().Format(time.RFC3339),
		})
	}
	s.cache = fresh // drop entries for files no longer scanned
	s.pruneLineage(regByProc)
	return dedupeBySession(reports), nil
}

// indexByProc maps each live process identity to the session id it currently
// runs, from the registry. Records without a usable pid/start-time are skipped,
// so a schema drift never forges a lineage link (#367).
func indexByProc(reg map[string]sessionRecord) map[procID]string {
	m := make(map[procID]string, len(reg))
	for id, rec := range reg {
		if rec.PID > 0 && rec.ProcStart != 0 {
			m[procID{pid: rec.PID, procStart: rec.ProcStart}] = id
		}
	}
	return m
}

// trackLineage returns the model and effort to report for a session, filling a
// fresh session's blanks from the last known values of the same process (a
// `/clear` inherits in place), and remembers this session's own readings for the
// next one on that process. Only live registry sessions take part — a superseded
// old transcript never donates or inherits (#367).
func (s *scanner) trackLineage(reg map[string]sessionRecord, id string, info *transcript.Info) (model, effort string) {
	model, effort = info.Model, info.Effort
	rec, live := reg[id]
	if !live || rec.PID <= 0 || rec.ProcStart == 0 {
		return model, effort
	}
	proc := procID{pid: rec.PID, procStart: rec.ProcStart}
	if prev, ok := s.lineage[proc]; ok {
		if model == "" {
			model = prev.model
		}
		if effort == "" {
			effort = prev.effort
		}
	}
	// Remember only real (parsed) values, so an inherited blank never sticks and
	// the entry tracks whichever session on this process last had its own reading.
	if info.Model != "" || info.Effort != "" {
		e := s.lineage[proc]
		if info.Model != "" {
			e.model = info.Model
		}
		if info.Effort != "" {
			e.effort = info.Effort
		}
		s.lineage[proc] = e
	}
	return model, effort
}

// pruneLineage drops lineage entries whose process is no longer live, keeping the
// table bounded to the processes currently in the registry (#367).
func (s *scanner) pruneLineage(regByProc map[procID]string) {
	for proc := range s.lineage {
		if _, live := regByProc[proc]; !live {
			delete(s.lineage, proc)
		}
	}
}

// parse returns the transcript Info for p. An unchanged file (same mod time and
// size) reuses the cached parser with no I/O; a grown file (append-only) resumes
// the cached parser from its offset and folds in only the new bytes; anything
// else (a shrunk/rewritten file, or first sight) is parsed from scratch (#257).
func (s *scanner) parse(p string, fi os.FileInfo, fresh map[string]cacheEntry) (*transcript.Info, error) {
	e, ok := s.cache[p]
	if ok && e.modTime.Equal(fi.ModTime()) && e.size == fi.Size() {
		fresh[p] = e
		return e.parser.Info(), nil
	}
	parser := transcript.NewParser()
	if ok && fi.Size() > e.size { // append-only growth: resume from where we stopped
		parser = e.parser
	}
	f, err := os.Open(p) //nolint:gosec // path comes from our own transcript glob
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(parser.Offset(), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seeking transcript: %w", err)
	}
	if err := parser.Advance(f); err != nil {
		return nil, err
	}
	fresh[p] = cacheEntry{modTime: fi.ModTime(), size: fi.Size(), parser: parser}
	return parser.Info(), nil
}

// dedupeBySession keeps one report per session id, the one carrying the largest
// output-token total.
//
// A transcript lives under ~/.claude/projects/<encoded-cwd>/, so a session whose
// working directory changes — a renamed or moved project, a resume from another
// path — is written under two directories under the *same* id. The glob then
// yielded one report per file, and the server saw the session's total alternate
// between the live figure and the abandoned file's, every scan. Downstream that
// read as the whole total being produced afresh each time (#432,
// docs/design/token-rollup.md).
//
// The largest total is the live file: an abandoned transcript stops growing.
// Reports without a session id are left alone rather than collapsed together.
func dedupeBySession(reports []api.ReportRequest) []api.ReportRequest {
	best := make(map[string]int, len(reports))
	out := make([]api.ReportRequest, 0, len(reports))
	for _, r := range reports {
		if r.SessionID == "" {
			out = append(out, r)
			continue
		}
		i, seen := best[r.SessionID]
		if !seen {
			best[r.SessionID] = len(out)
			out = append(out, r)
			continue
		}
		if outputTokens(r) > outputTokens(out[i]) {
			out[i] = r
		}
	}
	return out
}

func outputTokens(r api.ReportRequest) int64 {
	if r.Usage == nil {
		return 0
	}
	return r.Usage.OutputTokens
}

// noteUnknownStatuses tells the operator about each registry status word this
// build does not know, once per word.
//
// The word comes from another program, so it is capped and quoted on the way out:
// %q escapes control characters, and the cap stops a schema that stopped being an
// enum from writing a paragraph into the log. The same care a desktop
// notification gets, for the same reason (#529).
func (s *scanner) noteUnknownStatuses(reg map[string]sessionRecord) {
	for _, word := range unknownRegistryStatuses(reg) {
		if s.warnedStatus[word] {
			continue
		}
		s.warnedStatus[word] = true
		fmt.Fprintf(s.errw, "watch: claude code reported the session status %q, "+
			"which this build does not know; sessions carrying it are shown as idle "+
			"— this machine may need an upgrade\n", capText(word, 40))
	}
}
