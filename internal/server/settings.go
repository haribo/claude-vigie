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

// minRetention is the shortest window the API will store. Below it the setting
// stops being a retention policy and becomes a delete button: the prune loop
// takes `now - retention` as its cutoff and removes every session, event and
// token sample older than that — live sessions included, since the predicate is
// last-report time and not status.
//
// This is not a security control. Anyone who can set it holds the fleet token,
// and a token holder can already make the board lie in other ways
// ([deployment.md](../../docs/deployment.md) says what that reaches). It guards a
// *mistake*: Go durations have no month unit, so an operator who means thirty
// days and types `30s` rather than `720h` deletes their own history, with nothing
// between the keystroke and the deletion.
//
// One hour sits far below the smallest window the TUI offers — 24 h — and far
// above anything that could be meant seriously (#558).
const minRetention = time.Hour

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
		d, err := time.ParseDuration(req.SessionRetention)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid duration")
			return
		}
		if d < minRetention {
			s.writeError(w, http.StatusBadRequest, "session retention must be at least "+minRetention.String())
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
