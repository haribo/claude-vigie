// This file holds the watcher's HTTP transport: the one client, the request
// helpers every call goes through, and the error type that lets a caller tell a
// refusal from an outage.
//
// It is here rather than in watch.go because that file carried five jobs at once
// — the scan loop, transcript reading, status derivation, this transport, and the
// usage loop — and the one that matters is the status derivation: the rules that
// decide whether a session is working, waiting or idle, and where #190, #201,
// #233 and #512 all lived. `docs/code.md` asks for one responsibility per
// function and one purpose per package; a file mixing HTTP plumbing into that
// machine is the part of it that was cheapest to undo (#582).
//
// A move, nothing more. No signature changed and no behavior with it — the
// larger split of watch.go stays deferred on #379's verdict: do it when a feature
// actually collides with the file, not as a speculative big-bang.

package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/reachability"
	"github.com/haribo/claude-vigie/internal/version"
)

// httpClient carries a timeout (http.DefaultClient has none); each request also
// sets a context deadline.
var httpClient = &http.Client{Timeout: 10 * time.Second}

func post(cfg *config.Config, req api.ReportRequest) error {
	return postJSON(cfg, "/api/report", req, nil)
}

// httpError carries a non-2xx response so callers can act on the status, not just
// log it: the daemon answers 409 to a watch report whose build does not match its
// own (#384).
type httpError struct {
	status     int
	statusLine string
	msg        string
}

func (e *httpError) Error() string {
	if e.msg != "" {
		return fmt.Sprintf("server returned %s: %s", e.statusLine, e.msg)
	}
	return fmt.Sprintf("server returned %s", e.statusLine)
}

// isDrift reports whether err is the daemon refusing this watcher's build (#384).
func isDrift(err error) bool {
	var he *httpError
	return errors.As(err, &he) && he.status == http.StatusConflict
}

// serverErrorMessage extracts the daemon's {"error": "..."} message, or "" when
// the body carries none.
func serverErrorMessage(resp *http.Response) string {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ""
	}
	return body.Error
}

// getJSON GETs path from the server (with auth) and decodes it into out.
func getJSON(cfg *config.Config, path string, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(cfg.ServerURL, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return noteReachability(cfg, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_ = noteReachability(cfg, nil)
	if resp.StatusCode != http.StatusOK {
		return &httpError{status: resp.StatusCode, statusLine: resp.Status}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// reportDaemonDrift probes the daemon's build once at startup so a mismatch is
// named immediately, with its remediation, instead of only surfacing later as
// refused reports. Unknown is not drifted: an unreachable or erroring server
// leaves the watcher running and lets the daemon arbitrate per report (#384).
func reportDaemonDrift(cfg *config.Config) {
	var v api.VersionInfo
	if err := getJSON(cfg, "/api/version", &v); err != nil {
		return
	}
	if version.Match(version.Version, version.Commit, v.Version, v.Commit) {
		return
	}
	fmt.Fprintf(os.Stderr, "watch: this watcher is %s but the daemon is %s — session reports will be refused until this machine is upgraded\n",
		version.Describe(version.Version, version.Commit), version.Describe(v.Version, v.Commit))
}

// postJSON POSTs body to path on the server (with auth) and, if out is
// non-nil, decodes the response into it.
// noteReachability records what this request just learned about the daemon, for
// the reporting hooks to read (docs/design/unreachable-daemon.md, #578). A
// transport error means unreachable; any answer — including a refusal — means
// reachable.
//
// It lives here rather than on the heartbeat because the heartbeat is not
// reliably every 5 s: during an outage each session report waits out its own
// deadline first, so a machine with a dozen live sessions can take longer than
// the mark's window to come back round to beating. Every request refreshing it
// makes the mark independent of how long a scan cycle happens to take.
//
// Best-effort: the mark is an optimisation for the hooks, and failing to write it
// costs one deadline, never a report. The watcher never reads it — it is
// long-lived, so waiting out a deadline delays nobody, and it must keep probing
// or nothing would ever clear the mark.
func noteReachability(cfg *config.Config, err error) error {
	if err != nil {
		_ = reachability.Mark(cfg.ServerURL, clock.Now(), err)
		return err
	}
	_ = reachability.Clear(cfg.ServerURL)
	return nil
}

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
		return noteReachability(cfg, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_ = noteReachability(cfg, nil)
	if resp.StatusCode >= http.StatusMultipleChoices {
		return &httpError{status: resp.StatusCode, statusLine: resp.Status, msg: serverErrorMessage(resp)}
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}
