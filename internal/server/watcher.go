package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
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

// watchSeenKey is the meta key holding the RFC3339 time of the last watch report.
const watchSeenKey = "watch_seen"

// machineWatchKey is the meta key holding the RFC3339 time of the last watch
// report from a given machine (#284).
func machineWatchKey(machine string) string { return watchSeenKey + ":" + machine }

// machineWatchVersionKey holds a machine's watcher build as "version\tcommit" (#356).
func machineWatchVersionKey(machine string) string { return "watch_version:" + machine }

// handleWatcher returns when the server last received a watch report — globally
// and per machine — so the client can flag machines whose statuses may be stale
// because no watcher is running there.
func (s *Server) handleWatcher(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	seen, _, err := s.store.GetMeta(ctx, watchSeenKey)
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
	// One entry per machine that currently has sessions; "" means no watcher
	// heartbeat for it (reporting on hooks alone).
	machines := map[string]string{}
	versions := map[string]api.VersionInfo{}
	for _, sess := range sessions {
		if sess.Machine == "" {
			continue
		}
		if _, done := machines[sess.Machine]; done {
			continue
		}
		ts := ""
		if v, ok, mErr := s.store.GetMeta(ctx, machineWatchKey(sess.Machine)); mErr == nil && ok {
			ts = v
		}
		machines[sess.Machine] = ts
		if v, ok, mErr := s.store.GetMeta(ctx, machineWatchVersionKey(sess.Machine)); mErr == nil && ok {
			ver, commit, _ := strings.Cut(v, "\t")
			if ver != "" || commit != "" {
				versions[sess.Machine] = api.VersionInfo{Version: ver, Commit: commit}
			}
		}
	}
	resp := api.WatcherStatus{LastSeen: seen, Machines: machines}
	if len(versions) > 0 {
		resp.Versions = versions
	}
	s.writeJSON(w, http.StatusOK, resp)
}
