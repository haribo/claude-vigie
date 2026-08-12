package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/version"
)

// recordWatchHeartbeat records a watch report's liveness — globally and per
// machine — plus the reporting watcher's build, so clients can flag an absent
// (#284) or drifted (#356) watcher. Best-effort; errors are logged.
func (s *Server) recordWatchHeartbeat(ctx context.Context, req api.ReportRequest) {
	now := s.now().UTC().Format(time.RFC3339)
	if err := s.store.SetMeta(ctx, watchSeenKey, now); err != nil {
		s.log.Error("recording watch heartbeat", "error", err)
	}
	if req.Machine == "" {
		return
	}
	if err := s.store.SetMeta(ctx, machineWatchKey(req.Machine), now); err != nil {
		s.log.Error("recording per-machine watch heartbeat", "error", err, "machine", req.Machine)
	}
	if req.WatcherVersion != "" || req.WatcherCommit != "" {
		if err := s.store.SetMeta(ctx, machineWatchVersionKey(req.Machine), req.WatcherVersion+"\t"+req.WatcherCommit); err != nil {
			s.log.Error("recording per-machine watcher version", "error", err, "machine", req.Machine)
		}
	}
}

// watcherBuildMatches reports whether a watch report's declared build matches this
// daemon's, by the fleet's single version rule (internal/version). A report that
// declares no build at all comes from a watcher older than #356: it cannot identify
// itself, so it cannot satisfy an exact-match contract and counts as drifted
// (docs/design/version-consistency.md, #384).
func watcherBuildMatches(req api.ReportRequest) bool {
	if req.WatcherVersion == "" && req.WatcherCommit == "" {
		return false
	}
	return version.Match(req.WatcherVersion, req.WatcherCommit, version.Version, version.Commit)
}

// driftMessage names both builds and the remediation, so the operator learns which
// machine to upgrade and to what.
func driftMessage(req api.ReportRequest) string {
	return fmt.Sprintf("this watcher reports %s, which does not match this daemon (%s) — upgrade vigie on machine %q to match",
		version.Describe(req.WatcherVersion, req.WatcherCommit),
		version.Describe(version.Version, version.Commit),
		req.Machine)
}

// watchSeenKey is the meta key holding the RFC3339 time of the last watch report.
const watchSeenKey = "watch_seen"

// machineWatchKey is the meta key holding the RFC3339 time of the last watch
// report from a given machine (#284).
func machineWatchKey(machine string) string { return watchSeenKey + ":" + machine }

// machineWatchVersionKey holds a machine's watcher build as "version\tcommit" (#356).
func machineWatchVersionKey(machine string) string { return "watch_version:" + machine }

// handleWatcherHeartbeat records a watcher's liveness claim, independent of any
// session it may or may not have to report — a watcher with nothing to report is
// still running, and deriving liveness from session data hid exactly those
// machines (docs/design/watcher-liveness.md, #386).
//
// The heartbeat is always recorded, drifted or not: that is what keeps a refused
// machine visible. A drifted build still gets 409, which is also the answer that
// tells the watcher when it may resume (#384).
func (s *Server) handleWatcherHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req api.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Machine == "" {
		s.writeError(w, http.StatusBadRequest, "machine is required")
		return
	}

	// Reuse the report shape so liveness and the version gate share one rule.
	claim := api.ReportRequest{
		Machine: req.Machine, WatcherVersion: req.WatcherVersion, WatcherCommit: req.WatcherCommit,
	}
	ctx := r.Context()
	s.recordWatchHeartbeat(ctx, claim)
	if !watcherBuildMatches(claim) {
		s.writeError(w, http.StatusConflict, driftMessage(claim))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleWatcher returns when the server last received a watch report — globally
// and per machine — so the client can flag machines whose statuses may be stale
// because no watcher is running there.
func (s *Server) handleWatcher(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	meta, err := s.store.ListMeta(ctx)
	if err != nil {
		s.log.Error("reading watch heartbeat", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	sessions, err := s.store.ListSessions(ctx)
	if err != nil {
		s.log.Error("listing sessions for watcher status", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Machines come from two sources: those that currently have sessions, and
	// those the server has merely heard a watch report from. A machine whose
	// watcher is drifted writes no sessions at all (#384), so deriving the list
	// from sessions alone would hide exactly the machine the operator must see.
	names := map[string]bool{}
	for _, sess := range sessions {
		if sess.Machine != "" {
			names[sess.Machine] = true
		}
	}
	for k := range meta {
		if m, ok := strings.CutPrefix(k, watchSeenKey+":"); ok && m != "" {
			names[m] = true
		}
	}

	// "" as a timestamp means no watcher heartbeat for that machine (it is
	// reporting on hooks alone).
	machines := map[string]string{}
	versions := map[string]api.VersionInfo{}
	for name := range names {
		machines[name] = meta[machineWatchKey(name)]
		ver, commit, _ := strings.Cut(meta[machineWatchVersionKey(name)], "\t")
		if ver != "" || commit != "" {
			versions[name] = api.VersionInfo{Version: ver, Commit: commit}
		}
	}
	resp := api.WatcherStatus{LastSeen: meta[watchSeenKey], Machines: machines}
	if len(versions) > 0 {
		resp.Versions = versions
	}
	s.writeJSON(w, http.StatusOK, resp)
}
