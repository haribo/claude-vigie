package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
)

// RetentionMetaKey is the meta key holding the session-retention window (a Go
// duration string; "" disables pruning). Shared with the daemon's prune loop.
const RetentionMetaKey = "session_retention"

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	v, _, err := s.store.GetMeta(r.Context(), RetentionMetaKey)
	if err != nil {
		s.log.Error("reading settings", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.writeJSON(w, http.StatusOK, api.Settings{SessionRetention: v})
}

func (s *Server) handleSetSettings(w http.ResponseWriter, r *http.Request) {
	var req api.Settings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.SessionRetention != "" {
		if _, err := time.ParseDuration(req.SessionRetention); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid duration")
			return
		}
	}
	if err := s.store.SetMeta(r.Context(), RetentionMetaKey, req.SessionRetention); err != nil {
		s.log.Error("writing settings", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
