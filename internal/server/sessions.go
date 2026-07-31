package server

import (
	"net/http"
	"time"

	"github.com/haribo/claude-fleet/internal/api"
	"github.com/haribo/claude-fleet/internal/store"
)

// staleReportAfter is how long a session may go without a report before it is
// shown as ended: the watcher re-reports every scan (~2s), so a live session
// stays well within this, while one that dropped out of scan settles to ended.
const staleReportAfter = 60 * time.Second

// activityWindow bounds the ACT sparkline to recent samples, so an idle session
// (which stops producing samples) renders an empty graph instead of a frozen one.
const activityWindow = 15 * time.Minute

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.store.ListSessions(r.Context())
	if err != nil {
		s.log.Error("listing sessions", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := s.now()
	since := now.Add(-activityWindow).UTC().Format(time.RFC3339)
	views := make([]api.SessionView, 0, len(sessions))
	for _, ss := range sessions {
		samples, err := s.store.ListSamples(r.Context(), ss.ID, since, 30)
		if err != nil {
			s.log.Error("listing samples", "error", err)
		}
		views = append(views, toView(ss, samples, now))
	}
	s.writeJSON(w, http.StatusOK, views)
}

// reportStale reports whether the last report is too old (or never happened),
// meaning the session is no longer being refreshed and should read as ended.
func reportStale(reportedAt string, now time.Time) bool {
	if reportedAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, reportedAt)
	if err != nil {
		return false
	}
	return now.Sub(t) > staleReportAfter
}

func toView(s store.Session, samples []int64, now time.Time) api.SessionView {
	status := s.Status
	if s.Status != "ended" && reportStale(s.ReportedAt, now) {
		status = "ended"
	}
	return api.SessionView{
		ID:         s.ID,
		Title:      s.Title,
		User:       s.User,
		Machine:    s.Machine,
		ProjectDir: s.ProjectDir,
		GitBranch:  s.GitBranch,
		Model:      s.Model,
		Status:     status,
		LastTool:   s.LastTool,
		Usage: api.Usage{
			InputTokens:         s.Usage.InputTokens,
			OutputTokens:        s.Usage.OutputTokens,
			CacheCreationTokens: s.Usage.CacheCreationTokens,
			CacheReadTokens:     s.Usage.CacheReadTokens,
		},
		StartedAt:      s.StartedAt,
		LastSeenAt:     s.LastSeenAt,
		EndedAt:        s.EndedAt,
		RemoteControl:  s.RemoteControl,
		APIErrorStatus: s.APIErrorStatus,
		Activity:       s.Activity,
		Samples:        samples,
	}
}
