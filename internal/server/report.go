package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/store"
)

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	var req api.ReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			metricReportsRejected.WithLabelValues("too_large").Inc()
			s.writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		metricReportsRejected.WithLabelValues("bad_json").Inc()
		s.writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.SessionID == "" || req.Event == "" {
		metricReportsRejected.WithLabelValues("missing_fields").Inc()
		s.writeError(w, http.StatusBadRequest, "session_id and event are required")
		return
	}

	ctx := r.Context()
	existing, err := s.store.GetSession(ctx, req.SessionID)
	isNew := errors.Is(err, store.ErrNotFound)
	if err != nil && !isNew {
		s.log.Error("loading session", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	sess := applyReport(existing, isNew, req)
	sess.ReportedAt = s.now().UTC().Format(time.RFC3339) // server-side heartbeat
	if err := s.store.UpsertSession(ctx, sess); err != nil {
		s.log.Error("upserting session", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Tokens accrue on every report (analytics approach A): attribute the
	// output-token delta to today's daily rollup.
	s.rollupTokens(ctx, existing, sess, req)

	// The watcher polls frequently; keep its scans out of the event log, but
	// record a heartbeat so clients can detect an absent watcher.
	if req.Event == "watch" {
		if err := s.store.SetMeta(ctx, watchSeenKey, s.now().UTC().Format(time.RFC3339)); err != nil {
			s.log.Error("recording watch heartbeat", "error", err)
		}
	} else {
		// A hook event closes the previous status interval; roll up its
		// duration before recording the transition.
		s.rollupStatusInterval(ctx, sess, req)
		if err := s.store.AppendEvent(ctx, store.Event{
			SessionID: sess.ID, Event: req.Event, Status: sess.Status, CreatedAt: req.Timestamp,
		}); err != nil {
			// The session is already updated; the event log is best-effort.
			s.log.Error("appending event", "error", err)
		}
	}

	s.maybeSample(ctx, sess.ID, req.Timestamp, sess.Usage.OutputTokens)
	// Delta-gate the SSE fan-out (#258): the watcher re-reports every session
	// every ~2 s, mostly changing nothing but the heartbeat. Publish only when the
	// operator-visible state actually changed (or the session is new), so idle
	// clients stop refetching sessions/usage/stats/platform on every no-op scan.
	// The heartbeat still lives in ReportedAt; the stale→ended cutoff is evaluated
	// at read time, and the TUI's own 5 s tick refreshes relative ages.
	if isNew || visibleSignature(existing) != visibleSignature(sess) {
		s.hub.publish()
	}

	metricReports.WithLabelValues(req.Event).Inc()
	w.WriteHeader(http.StatusNoContent)
}

// rollupTokens adds the session's output-token growth since the last report to
// today's daily rollup, bucketed by model. Best-effort.
func (s *Server) rollupTokens(ctx context.Context, old, sess store.Session, req api.ReportRequest) {
	delta := sess.Usage.OutputTokens - old.Usage.OutputTokens
	if delta <= 0 {
		return
	}
	metricOutputTokens.WithLabelValues(sess.Model).Add(float64(delta))
	if err := s.store.AddDailyTokens(ctx, dayOf(req.Timestamp, s.now()), sess.Model, delta); err != nil {
		s.log.Error("rolling up daily tokens", "error", err)
	}
}

// rollupStatusInterval closes the interval since the session's previous event by
// adding its duration to that status's daily bucket. Only hook events carry
// status transitions, so the watcher's polls never reach here. Best-effort.
func (s *Server) rollupStatusInterval(ctx context.Context, sess store.Session, req api.ReportRequest) {
	last, ok, err := s.store.LastEvent(ctx, sess.ID)
	if err != nil || !ok {
		return
	}
	secs := secondsBetween(last.CreatedAt, req.Timestamp)
	if secs <= 0 {
		return
	}
	// Attribute the whole interval to its start day (no midnight split in v1).
	if err := s.store.AddDailyStatusSeconds(ctx, dayOf(last.CreatedAt, s.now()), sess.Model, last.Status, secs); err != nil {
		s.log.Error("rolling up daily status", "error", err)
	}
}

// dayOf returns the UTC calendar day (YYYY-MM-DD) of an RFC3339 timestamp,
// falling back to the current day when it cannot be parsed.
func dayOf(ts string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t = now
	}
	return t.UTC().Format("2006-01-02")
}

// secondsBetween returns the whole seconds from RFC3339 timestamp from to to,
// or 0 if either is unparseable or the span is non-positive.
func secondsBetween(from, to string) int64 {
	a, err1 := time.Parse(time.RFC3339, from)
	b, err2 := time.Parse(time.RFC3339, to)
	if err1 != nil || err2 != nil {
		return 0
	}
	if d := b.Sub(a); d > 0 {
		return int64(d.Seconds())
	}
	return 0
}

// maybeSample records a token sample at most once per minute per session, so
// the watcher's frequent reports don't flood the samples table.
func (s *Server) maybeSample(ctx context.Context, sessionID, at string, output int64) {
	if last, err := s.store.LastSampleAt(ctx, sessionID); err == nil && last != "" {
		lt, e1 := time.Parse(time.RFC3339, last)
		nt, e2 := time.Parse(time.RFC3339, at)
		if e1 == nil && e2 == nil && nt.Sub(lt) < time.Minute {
			return
		}
	}
	if err := s.store.AddSample(ctx, sessionID, at, output); err != nil {
		s.log.Error("adding sample", "error", err)
	}
}

// visibleSignature is a fingerprint of everything the dashboard shows for a
// session *except* the pure heartbeats (ReportedAt, LastSeenAt), which change on
// every report and are refreshed by the client's own tick. Two sessions with the
// same signature are indistinguishable on screen, so an SSE push between them is
// wasted (#258).
func visibleSignature(s store.Session) string {
	u := s.Usage
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%t|%s|%d|%d|%d|%d|%d",
		s.Status, s.StatusChangedAt, s.Activity, s.Title, s.User, s.Machine, s.Model,
		s.GitBranch, s.ProjectDir, s.LastTool, s.RemoteControl, s.RemoteURL, s.APIErrorStatus,
		u.InputTokens, u.OutputTokens, u.CacheCreationTokens, u.CacheReadTokens)
}

// applyReport merges a report into the session state (read-modify-write).
// Fields absent from the report are preserved, so an event without usage does
// not zero the accumulated tokens.
func applyReport(sess store.Session, isNew bool, req api.ReportRequest) store.Session {
	sess.ID = req.SessionID
	// Context fields are preserved when a later event omits them, so a partial
	// report (e.g. Stop without git_branch) does not erase known context.
	if req.User != "" {
		sess.User = req.User
	}
	if req.Machine != "" {
		sess.Machine = req.Machine
	}
	if req.ProjectDir != "" {
		sess.ProjectDir = req.ProjectDir
	}
	if req.GitBranch != "" {
		sess.GitBranch = req.GitBranch
	}
	if req.Model != "" {
		sess.Model = req.Model
	}
	if req.Effort != "" {
		sess.Effort = req.Effort
	}
	if req.Title != "" {
		sess.Title = req.Title
	}
	if isNew {
		sess.StartedAt = req.Timestamp
	}
	sess.LastSeenAt = req.Timestamp
	sess = applyStatus(sess, req)
	if req.Event == "SessionEnd" {
		sess.EndedAt = req.Timestamp
	}
	if req.RemoteControl != nil {
		sess.RemoteControl = *req.RemoteControl // detected /rc state (read-only)
		sess.RemoteURL = req.RemoteURL          // resume URL travels with the /rc flag; "" clears it
	}
	if req.LastTool != "" {
		sess.LastTool = req.LastTool
	}
	if req.Usage != nil {
		sess.Usage = store.Usage{
			InputTokens:         req.Usage.InputTokens,
			OutputTokens:        req.Usage.OutputTokens,
			CacheCreationTokens: req.Usage.CacheCreationTokens,
			CacheReadTokens:     req.Usage.CacheReadTokens,
		}
	}
	return sess
}

// applyStatus folds the report's status into the session: the reconciled status
// and its owner, when it last changed, and the transient activity message.
func applyStatus(sess store.Session, req api.ReportRequest) store.Session {
	prev := sess.Status
	if req.Status != "" {
		if !holdsWaiting(sess, req) {
			sess.Status, sess.StatusSource = reconcileWatch(sess.Status, sess.StatusSource, req.Status)
		}
		sess.APIErrorStatus = req.APIErrorStatus // watcher-derived; hooks carry no status
	} else {
		sess.Status = deriveStatus(req.Event, req.NotificationType, sess.Status)
		sess.StatusSource = "hook" // a hook event is the authoritative observer
	}
	if sess.Status != prev {
		sess.StatusChangedAt = req.Timestamp
	}
	// Activity describes an active turn; a resting status carries none (#236). So
	// clear it on idle/ended, take a fresh message when the report has one, else
	// drop a stale one on any status change so a new episode never shows the old
	// "doing".
	switch {
	case sess.Status == "idle" || sess.Status == "ended":
		sess.Activity = ""
	case req.Activity != "":
		sess.Activity = req.Activity
	case sess.Status != prev:
		sess.Activity = ""
	}
	return sess
}

// holdsWaiting reports whether a hook-set `waiting` must survive a watcher
// report (#235). The watcher can't tell "a tool is running" from "a permission
// prompt is blocking": both are a turn stopped on tool_use with a frozen
// transcript. So its inferred working/thinking may only clear waiting once the
// transcript has actually moved past when waiting was posted — i.e. the report's
// timestamp (the transcript mtime) is newer than StatusChangedAt. error/ended
// are positive observations and still win.
func holdsWaiting(sess store.Session, req api.ReportRequest) bool {
	if sess.Status != "waiting" || sess.StatusSource != "hook" || sess.StatusChangedAt == "" {
		return false
	}
	if req.Status != "working" && req.Status != "thinking" {
		return false
	}
	return !timeAfter(req.Timestamp, sess.StatusChangedAt)
}

// timeAfter reports whether RFC3339 time a is strictly after b (false on any
// parse error, so a missing timestamp never holds anything).
func timeAfter(a, b string) bool {
	ta, err1 := time.Parse(time.RFC3339, a)
	tb, err2 := time.Parse(time.RFC3339, b)
	if err1 != nil || err2 != nil {
		return false
	}
	return ta.After(tb)
}

// reconcileWatch folds a watcher-reported status into the session, resolving the
// overlap between the two observers by *authority* rather than a status table.
//
// The watcher polls the transcript and process; it is authoritative for coverage
// and for anything it can positively observe (working, thinking, error, ended).
// But it only ever sees a quiet-but-alive session as "idle", and it cannot see
// two things a hook can: that the operator is the blocker (waiting), or that a
// turn is open while Claude works silently. So the watcher's "idle" must not
// retract a *hook-owned* active state — yet it must retract its *own* stale
// state, which is the finished-session latch bug (#201): a watcher-set "working"
// has to fall back to idle when the transcript goes quiet.
//
// It returns the new status and its owning source.
func reconcileWatch(current, currentSource, incoming string) (status, source string) {
	if incoming == "idle" && currentSource == "hook" &&
		(current == "waiting" || current == "working" || current == "thinking") {
		return current, "hook" // keep the hook's semantic/active state
	}
	if incoming == current {
		return current, currentSource // a confirmation doesn't transfer ownership
	}
	return incoming, "watch" // a positive change is the watcher's
}

// deriveStatus maps a hook event to a session status, keeping the current
// status for events that do not change it. notifType is the Notification hook's
// notification_type; it splits "waiting on a human" from "idle".
func deriveStatus(event, notifType, current string) string {
	switch event {
	case "SessionStart":
		if current == "" {
			return "idle"
		}
		return current
	case "UserPromptSubmit":
		return "working"
	case "Notification":
		// A Notification means Claude wants attention. Only idle_prompt (finished,
		// awaiting the next prompt) is truly idle; permission_prompt and the rest
		// mean the operator is the blocker. Empty (older Claude Code) stays waiting.
		if notifType == "idle_prompt" {
			return "idle"
		}
		return "waiting"
	case "Stop":
		return "idle"
	case "SessionEnd":
		return "ended"
	default:
		// e.g. PostToolUse: activity, keep (or enter) the working state.
		if current == "" {
			return "working"
		}
		return current
	}
}
