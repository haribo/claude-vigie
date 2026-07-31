// Package watch continuously scans the local Claude Code transcripts and reports
// every recent session to the fleet server, deriving status from the transcript
// state. Client-side; it covers sessions the hooks miss (already-open ones).
package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/haribo/claude-fleet/internal/api"
	"github.com/haribo/claude-fleet/internal/clock"
	"github.com/haribo/claude-fleet/internal/config"
	"github.com/haribo/claude-fleet/internal/presence"
	"github.com/haribo/claude-fleet/internal/transcript"
	"github.com/haribo/claude-fleet/internal/usage"
)

// Options configures the watch loop.
type Options struct {
	Interval      time.Duration
	MaxAge        time.Duration
	UsageInterval time.Duration
}

// Status thresholds derived from how recently a transcript changed.
const (
	activeWindow = 10 * time.Second // transcript written this recently = working
	toolWindow   = 5 * time.Minute  // a tool_use turn may run this long before writing
)

// gcInterval is how often the watcher garbage-collects dead session mappings.
const gcInterval = 5 * time.Minute

// systemUser returns the OS account the watcher runs as (which, on a typical
// single-user machine, is the account that launched the sessions): the USER env
// var if set, else the current user, else "".
func systemUser() string {
	if u := config.OSUser(); u != "" {
		return u
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// ProjectsDir returns the Claude Code transcripts root (~/.claude/projects).
func ProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// Run scans on an interval and reports until ctx is canceled.
func Run(ctx context.Context, cfg *config.Config, opts Options) error {
	root, err := ProjectsDir()
	if err != nil {
		return err
	}

	go runUsageLoop(ctx, cfg, opts.UsageInterval)

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	sc := newScanner()
	lastGC := clock.Now()
	for {
		reports, err := sc.scan(root, cfg.Machine, opts.MaxAge, clock.Now())
		if err != nil {
			fmt.Fprintf(os.Stderr, "watch: %v\n", err)
		}
		for _, r := range reports {
			if err := post(cfg, r); err != nil {
				fmt.Fprintf(os.Stderr, "watch: reporting %s: %v\n", r.SessionID, err)
			}
		}
		if time.Since(lastGC) > gcInterval {
			collectDeadMappings(opts.MaxAge)
			lastGC = clock.Now()
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// collectDeadMappings removes presence mappings for sessions whose process died
// without a SessionEnd and whose transcript is past the watcher's window.
func collectDeadMappings(maxAge time.Duration) {
	n, err := presence.GC(maxAge, clock.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: presence gc: %v\n", err)
	} else if n > 0 {
		fmt.Fprintf(os.Stderr, "watch: cleaned %d dead session mapping(s)\n", n)
	}
}

// cacheEntry is a transcript's last parse, keyed by path and validated by the
// file's mod time and size.
type cacheEntry struct {
	modTime time.Time
	size    int64
	info    *transcript.Info
}

// scanner scans transcripts and caches each parse, so an unchanged (idle)
// transcript is not re-parsed every interval — important because a large
// transcript takes seconds to parse and the watcher scans frequently.
type scanner struct {
	cache map[string]cacheEntry
}

func newScanner() *scanner {
	return &scanner{cache: map[string]cacheEntry{}}
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
	rcMap := remoteControlled()
	var reports []api.ReportRequest
	fresh := make(map[string]cacheEntry, len(s.cache))
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		age := now.Sub(fi.ModTime())
		if age > maxAge {
			continue
		}
		info, err := s.parse(p, fi, fresh)
		if err != nil {
			continue
		}

		// Prefer the last dated transcript line over the file mtime for "when did
		// this session last really do something". A live Claude appends
		// untimestamped metadata (last-prompt, bridge-session) roughly hourly,
		// bumping mtime without any activity; LastActivity ignores those lines, so
		// SEEN and the age-based status stay truthful. The mtime still gates the
		// scan window above (the hourly churn keeps a live session in range, so it
		// stays visible as idle — never expired while its process lives). Fall back
		// to mtime when no dated line exists yet (a brand-new transcript).
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
		rc := rcMap[id]
		status := withThinking(sessionStatus(id, info.LastStopReason, info.LastAPIError, activityAge), info.Thinking)
		apiErr := 0
		if status == "error" {
			apiErr = info.LastAPIError // carry the HTTP code only while the error is shown
		}
		reports = append(reports, api.ReportRequest{
			Event:          "watch",
			SessionID:      id,
			User:           osUser,
			Machine:        machine,
			ProjectDir:     info.Cwd,
			GitBranch:      info.GitBranch,
			Model:          info.Model,
			Title:          info.Title,
			Status:         status,
			RemoteControl:  &rc,
			Usage:          &usage,
			APIErrorStatus: apiErr,
			Activity:       info.Activity,
			Timestamp:      lastActivity.UTC().Format(time.RFC3339),
		})
	}
	s.cache = fresh // drop entries for files no longer scanned
	return reports, nil
}

// parse returns the transcript Info for p, reusing the cached parse when the
// file is unchanged (same mod time and size) since the last scan. The reused or
// freshly parsed entry is recorded in fresh (the next scan's cache).
func (s *scanner) parse(p string, fi os.FileInfo, fresh map[string]cacheEntry) (*transcript.Info, error) {
	if e, ok := s.cache[p]; ok && e.modTime.Equal(fi.ModTime()) && e.size == fi.Size() {
		fresh[p] = e
		return e.info, nil
	}
	info, err := transcript.Parse(p)
	if err != nil {
		return nil, err
	}
	fresh[p] = cacheEntry{modTime: fi.ModTime(), size: fi.Size(), info: info}
	return info, nil
}

// sessionStatus layers a transient "error" status on top of the base
// derivation: when the last assistant line was an API error (500/529/429…), a
// live session — one that would otherwise read working or idle — reports error
// until a later non-error line clears it. A closed session (ended) is never
// shown as error, so a stale transcript does not stay red forever.
func sessionStatus(sessionID, lastStopReason string, lastAPIError int, age time.Duration) string {
	st := statusFor(sessionID, lastStopReason, age)
	if lastAPIError != 0 && (st == "working" || st == "idle") {
		return "error"
	}
	return st
}

// withThinking refines an active status to "thinking" when the transcript's last
// assistant block is a thinking block — Claude is reasoning inside the turn. It
// only refines working/idle (a live turn); error, ended, and waiting are left as
// is. Heuristic: at rest a completed turn's last block is text/tool, so this is
// true only mid-turn (a turn aborted right after thinking may briefly mis-show it
// until the next scan).
func withThinking(status string, thinking bool) string {
	if thinking && (status == "working" || status == "idle") {
		return "thinking"
	}
	return status
}

// statusFor derives a session's status from process presence and transcript
// activity:
//   - mapping present & dead → ended (reliable even on a hard kill)
//   - transcript actively changing → working (mapping or not)
//   - mapping present & alive but idle → idle (for any duration)
//   - no mapping & inactive → ended (presumed closed; a live session gets a
//     mapping via the SessionStart/UserPromptSubmit backfill)
func statusFor(sessionID, lastStopReason string, age time.Duration) string {
	m, ok, err := presence.Load(sessionID)
	hasMapping := err == nil && ok
	switch {
	case hasMapping && !presence.Alive(m):
		return "ended"
	case activelyWorking(lastStopReason, age):
		return "working"
	case hasMapping:
		return "idle"
	default:
		return "ended"
	}
}

// activelyWorking reports whether the transcript shows work in progress: it
// changed within activeWindow, or the last turn stopped on a tool call still
// within toolWindow (a long-running tool that has not written yet).
func activelyWorking(lastStopReason string, age time.Duration) bool {
	return age < activeWindow || (lastStopReason == "tool_use" && age < toolWindow)
}

// httpClient carries a timeout (http.DefaultClient has none); each request also
// sets a context deadline.
var httpClient = &http.Client{Timeout: 10 * time.Second}

func post(cfg *config.Config, req api.ReportRequest) error {
	return postJSON(cfg, "/api/report", req, nil)
}

// postJSON POSTs body to path on the server (with auth) and, if out is
// non-nil, decodes the response into it.
func postJSON(cfg *config.Config, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}
	url := strings.TrimRight(cfg.ServerURL, "/") + path

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// runUsageLoop periodically tries to hold the usage lease and, when it does,
// fetches subscription usage and reports it. The token never leaves the machine.
func runUsageLoop(ctx context.Context, cfg *config.Config, interval time.Duration) {
	if interval <= 0 {
		return
	}
	fetcher := &usage.Fetcher{}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		usageCycle(ctx, cfg, fetcher)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func usageCycle(ctx context.Context, cfg *config.Config, fetcher *usage.Fetcher) {
	var lease api.LeaseResponse
	if err := postJSON(cfg, "/api/usage/lease", api.LeaseRequest{Holder: cfg.Machine}, &lease); err != nil {
		fmt.Fprintf(os.Stderr, "watch: usage lease: %v\n", err)
		return
	}
	if !lease.Acquired {
		return // another machine holds the lease
	}
	rep, ok, err := fetcher.Fetch(ctx, clock.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: usage fetch: %v\n", err)
		return
	}
	if !ok {
		return // backing off
	}
	if err := postJSON(cfg, "/api/usage", rep, nil); err != nil {
		fmt.Fprintf(os.Stderr, "watch: post usage: %v\n", err)
	}
}
