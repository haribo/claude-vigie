package server

import (
	"net/http"

	"github.com/haribo/claude-fleet/internal/api"
	"github.com/haribo/claude-fleet/internal/store"
)

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.store.ListSessions(r.Context())
	if err != nil {
		s.log.Error("listing sessions", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	views := make([]api.SessionView, 0, len(sessions))
	for _, ss := range sessions {
		samples, err := s.store.ListSamples(r.Context(), ss.ID, 30)
		if err != nil {
			s.log.Error("listing samples", "error", err)
		}
		views = append(views, toView(ss, samples))
	}
	s.writeJSON(w, http.StatusOK, views)
}

func toView(s store.Session, samples []int64) api.SessionView {
	return api.SessionView{
		ID:         s.ID,
		Title:      s.Title,
		User:       s.User,
		Machine:    s.Machine,
		ProjectDir: s.ProjectDir,
		GitBranch:  s.GitBranch,
		Model:      s.Model,
		Status:     s.Status,
		LastTool:   s.LastTool,
		Usage: api.Usage{
			InputTokens:         s.Usage.InputTokens,
			OutputTokens:        s.Usage.OutputTokens,
			CacheCreationTokens: s.Usage.CacheCreationTokens,
			CacheReadTokens:     s.Usage.CacheReadTokens,
		},
		StartedAt:     s.StartedAt,
		LastSeenAt:    s.LastSeenAt,
		EndedAt:       s.EndedAt,
		RemoteControl: s.RemoteControl,
		Samples:       samples,
	}
}
