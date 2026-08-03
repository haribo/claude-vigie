package server

import (
	"net/http"

	"github.com/haribo/claude-vigie/internal/api"
)

// watchSeenKey is the meta key holding the RFC3339 time of the last watch report.
const watchSeenKey = "watch_seen"

// machineWatchKey is the meta key holding the RFC3339 time of the last watch
// report from a given machine (#284).
func machineWatchKey(machine string) string { return watchSeenKey + ":" + machine }

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
	}
	s.writeJSON(w, http.StatusOK, api.WatcherStatus{LastSeen: seen, Machines: machines})
}
