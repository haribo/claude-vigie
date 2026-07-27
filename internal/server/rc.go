package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/haribo/claude-fleet/internal/api"
	"github.com/haribo/claude-fleet/internal/store"
)

// handleSetRC toggles a session's remote-control flag. This is the first
// client→server write path beyond reporting.
func (s *Server) handleSetRC(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "missing session id")
		return
	}
	var req api.RCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := s.store.SetRemoteControl(r.Context(), id, req.Enabled); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.writeError(w, http.StatusNotFound, "session not found")
			return
		}
		s.log.Error("setting remote control", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.hub.publish() // nudge watchers/TUIs to refresh
	w.WriteHeader(http.StatusNoContent)
}
