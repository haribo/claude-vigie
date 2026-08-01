package server

import (
	"net/http"

	"github.com/haribo/claude-vigie/internal/api"
)

// watchSeenKey is the meta key holding the RFC3339 time of the last watch report.
const watchSeenKey = "watch_seen"

// handleWatcher returns when the server last received a watch report, so the
// client can warn that statuses may be stale when no watcher is running.
func (s *Server) handleWatcher(w http.ResponseWriter, r *http.Request) {
	seen, _, err := s.store.GetMeta(r.Context(), watchSeenKey)
	if err != nil {
		s.log.Error("reading watch heartbeat", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.writeJSON(w, http.StatusOK, api.WatcherStatus{LastSeen: seen})
}
