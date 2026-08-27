package server

import (
	"context"
	"math"
	"net/http"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/modelinfo"
	"github.com/haribo/claude-vigie/internal/status"
	"github.com/haribo/claude-vigie/internal/store"
)

// contextPctView is how full the session's context window is, rounded to a whole
// percent, or nil when there is no reading at all.
//
// The daemon rounds, once, so that every client displays the same number because
// it *is* the same number. Deriving the percentage per client meant deriving the
// rounding too, and Go's %.0f rounds half to even where JavaScript's Math.round
// rounds half up — a divergence the shared fixture used to steer around rather
// than remove (ADR-0011, #616).
func contextPctView(tokens int64, known bool, model string) *int {
	if !known {
		return nil
	}
	pct := 0
	if tokens > 0 {
		pct = int(math.Round(float64(tokens) / float64(modelinfo.Window(model)) * 100))
	}
	return &pct
}

// contextView maps the stored context reading onto the view's pointer: nil when
// the count is not known (rendered "-"), else a pointer to the count — including
// a known 0 for a just-cleared session shown as 0% (#367).
func contextView(tokens int64, known bool) *int64 {
	if !known {
		return nil
	}
	return &tokens
}

// staleReportAfter is how long a session may go without a report before it is
// shown as ended: the watcher re-reports every scan (~2s), so a live session
// stays well within this, while one that dropped out of scan settles to ended.
const staleReportAfter = 60 * time.Second

// activityWindow bounds the ACT sparkline to recent samples, so an idle session
// (which stops producing samples) renders an empty graph instead of a frozen one.
const activityWindow = 15 * time.Minute

// sampleWindow is how many points one sparkline may draw. The cap is applied per
// session by the query, so a busy session cannot crowd a quiet one out.
const sampleWindow = 30

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
	// One read for the whole board, not one per session: this route is what the
	// TUI, the browser and the GNOME indicator all fetch on every change, so a
	// query per row multiplied by every client on every change (#580). A failure
	// costs the sparklines and nothing else, so it is logged and the board is
	// still served.
	samples, err := s.store.RecentSamples(r.Context(), since, sampleWindow)
	if err != nil {
		s.log.Error("listing recent samples", "error", err)
		samples = nil
	}
	views := make([]api.SessionView, 0, len(sessions))
	for _, ss := range sessions {
		views = append(views, toView(ss, samples[ss.ID], now, watched[ss.Machine]))
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
	// The effective status, which is not always the stored one — and is what every
	// derived field below must read. Naming it `status` shadowed the package of the
	// same name and hid that: Attention and Rank taken from s.Status would have
	// ranked a session by a status the client is never shown, leaving a stale
	// session sorted among the working ones (#617).
	effective := s.Status
	if s.Status != "ended" && reportStale(s.ReportedAt, now) {
		// No fresh report. A watched machine's watcher would have kept a live
		// session fresh, so a stale one there is genuinely gone → ended. On an
		// unwatched machine "no news" means "unobserved", not "dead" → stale (#285).
		if machineWatched {
			effective = "ended"
		} else {
			effective = "stale"
		}
	}
	return api.SessionView{
		ID:             s.ID,
		Title:          s.Title,
		Name:           nameView(s.Title, s.ID),
		User:           s.User,
		Machine:        s.Machine,
		ProjectDir:     s.ProjectDir,
		Project:        projectView(s.ProjectDir),
		GitBranch:      s.GitBranch,
		Model:          s.Model,
		ModelShort:     modelinfo.Short(s.Model),
		Effort:         s.Effort,
		ContextTokens:  contextView(s.ContextTokens, s.ContextKnown),
		Attention:      status.NeedsAttention(effective),
		Rank:           status.Rank(effective),
		ContextWindow:  modelinfo.Window(s.Model),
		ContextPct:     contextPctView(s.ContextTokens, s.ContextKnown, s.Model),
		PermissionMode: s.PermissionMode,
		ModeLabel:      modeLabelView(s.PermissionMode),
		ModeDetail:     modeDetailView(s.PermissionMode),
		Status:         effective,
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
		Detail:          s.Detail,
		DetailText:      detailTextView(s, effective),
		StatusChangedAt: s.StatusChangedAt,
		Samples:         samples,
		CallAt:          s.CallAt,
		CallMessage:     s.CallMessage,
	}
}
