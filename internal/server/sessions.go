package server

import (
	"context"
	"net/http"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
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
	watched := watchedMachines(r.Context(), s.store, sessions, now)
	views := make([]api.SessionView, 0, len(sessions))
	for _, ss := range sessions {
		samples, err := s.store.ListSamples(r.Context(), ss.ID, since, 30)
		if err != nil {
			s.log.Error("listing samples", "error", err)
		}
		views = append(views, toView(ss, samples, now, watched[ss.Machine]))
	}
	s.writeJSON(w, http.StatusOK, views)
}

// reportStale reports whether the last report is too old (or never happened),
// meaning the session is no longer being refreshed.
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

// metaReader reads meta values — here, the per-machine watch heartbeats.
type metaReader interface {
	GetMeta(ctx context.Context, key string) (string, bool, error)
}

// watchedMachines returns which machines have a recent watcher heartbeat, so a
// stale session on an unwatched machine can read as "stale" rather than a false
// "ended" (#285). The freshness window matches the session stale net.
func watchedMachines(ctx context.Context, r metaReader, sessions []store.Session, now time.Time) map[string]bool {
	watched := map[string]bool{}
	for _, ss := range sessions {
		m := ss.Machine
		if m == "" {
			continue
		}
		if _, done := watched[m]; done {
			continue
		}
		fresh := false
		if ts, ok, _ := r.GetMeta(ctx, machineWatchKey(m)); ok {
			if t, err := time.Parse(time.RFC3339, ts); err == nil && now.Sub(t) <= staleReportAfter {
				fresh = true
			}
		}
		watched[m] = fresh
	}
	return watched
}

func toView(s store.Session, samples []int64, now time.Time, machineWatched bool) api.SessionView {
	status := s.Status
	if s.Status != "ended" && reportStale(s.ReportedAt, now) {
		// No fresh report. A watched machine's watcher would have kept a live
		// session fresh, so a stale one there is genuinely gone → ended. On an
		// unwatched machine "no news" means "unobserved", not "dead" → stale (#285).
		if machineWatched {
			status = "ended"
		} else {
			status = "stale"
		}
	}
	return api.SessionView{
		ID:             s.ID,
		Title:          s.Title,
		User:           s.User,
		Machine:        s.Machine,
		ProjectDir:     s.ProjectDir,
		GitBranch:      s.GitBranch,
		Model:          s.Model,
		Effort:         s.Effort,
		ContextTokens:  s.ContextTokens,
		PermissionMode: s.PermissionMode,
		Status:         status,
		LastTool:       s.LastTool,
		Usage: api.Usage{
			InputTokens:         s.Usage.InputTokens,
			OutputTokens:        s.Usage.OutputTokens,
			CacheCreationTokens: s.Usage.CacheCreationTokens,
			CacheReadTokens:     s.Usage.CacheReadTokens,
		},
		StartedAt:       s.StartedAt,
		LastSeenAt:      s.LastSeenAt,
		EndedAt:         s.EndedAt,
		RemoteControl:   s.RemoteControl,
		RemoteURL:       s.RemoteURL,
		APIErrorStatus:  s.APIErrorStatus,
		Activity:        s.Activity,
		StatusChangedAt: s.StatusChangedAt,
		Samples:         samples,
	}
}
