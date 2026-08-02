package server

import (
	"net/http"
	"sort"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
)

// topSessionsLimit bounds the ranked sessions returned by the stats endpoint.
const topSessionsLimit = 10

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	daily, err := s.store.ListDailyStats(ctx, r.URL.Query().Get("since"))
	if err != nil {
		s.log.Error("listing daily stats", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	sessions, err := s.store.ListSessions(ctx)
	if err != nil {
		s.log.Error("listing sessions", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.writeJSON(w, http.StatusOK, api.StatsResponse{
		Daily:        toDailyStats(daily),
		TopSessions:  topSessions(sessions),
		SessionCount: len(sessions),
	})
}

func toDailyStats(rows []store.DailyStat) []api.DailyStat {
	out := make([]api.DailyStat, 0, len(rows))
	for _, d := range rows {
		out = append(out, api.DailyStat{
			Day:            d.Day,
			Model:          d.Model,
			OutputTokens:   d.OutputTokens,
			WorkingSeconds: d.WorkingSeconds,
			WaitingSeconds: d.WaitingSeconds,
			IdleSeconds:    d.IdleSeconds,
		})
	}
	return out
}

// topSessions ranks sessions by output tokens (descending) and returns the top
// few, naming each by its title when set, else its id (a hash).
func topSessions(sessions []store.Session) []api.TopSession {
	sorted := make([]store.Session, len(sessions))
	copy(sorted, sessions)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Usage.OutputTokens > sorted[j].Usage.OutputTokens
	})
	if len(sorted) > topSessionsLimit {
		sorted = sorted[:topSessionsLimit]
	}
	out := make([]api.TopSession, 0, len(sorted))
	for _, ss := range sorted {
		name := ss.Title
		if name == "" {
			name = ss.ID
		}
		out = append(out, api.TopSession{
			Name:         name,
			Machine:      ss.Machine,
			Model:        ss.Model,
			Status:       ss.Status,
			OutputTokens: ss.Usage.OutputTokens,
		})
	}
	return out
}
